"""Black-box FileShare adapter checks. Writes only into the supplied workspace _temp tree."""
import argparse
import concurrent.futures
import http.client
import json
import pathlib
import socket
import subprocess
import uuid
from urllib.parse import urlsplit, parse_qs


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--exe', required=True)
    parser.add_argument('--work', required=True)
    args = parser.parse_args()
    root = pathlib.Path(__file__).resolve().parents[1]
    work = pathlib.Path(args.work).resolve()
    if not work.is_relative_to(root / '_temp'):
        raise ValueError('测试目录必须位于仓库 _temp 下')
    work = work / ('lan-' + uuid.uuid4().hex)
    work.mkdir(parents=True)
    original = work / '原始文件.txt'
    original.write_bytes(b'0123456789abcdef')
    received = work / 'received'
    config = {'port': 0, 'bind': '127.0.0.1', 'dataDir': str(work / 'data'), 'downloadDir': str(received)}
    process = subprocess.Popen([str(pathlib.Path(args.exe).resolve())], stdin=subprocess.PIPE,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding='utf-8',
                               creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
    sequence = 0
    reads = concurrent.futures.ThreadPoolExecutor(max_workers=1)

    def rpc(method, params=None, error=False):
        nonlocal sequence
        sequence += 1
        process.stdin.write(json.dumps({'id': str(sequence), 'method': method, 'params': params}) + '\n')
        process.stdin.flush()
        line = reads.submit(process.stdout.readline).result(timeout=12)
        if not line:
            raise AssertionError('共享进程未返回响应: ' + process.stderr.read())
        reply = json.loads(line)
        assert reply['id'] == str(sequence), reply
        assert ('error' in reply) == error, reply
        return reply.get('result')

    try:
        rpc('start', {**config, 'dataDir': 'relative'}, error=True)
        info = rpc('start', config)
        port = info['port']
        key = parse_qs(urlsplit(info['url']).query)['key'][0]
        headers = {'Authorization': 'Bearer ' + key}

        def request(path, method='GET', body=None, extra=None, authorized=True, expected=200):
            conn = http.client.HTTPConnection('127.0.0.1', port, timeout=5)
            h = {**(headers if authorized else {}), **(extra or {})}
            conn.request(method, path, body, h)
            response = conn.getresponse()
            data, response_headers = response.read(), dict(response.getheaders())
            assert response.status == expected, (path, response.status, data)
            conn.close()
            return data, response_headers

        request('/api/items', authorized=False, expected=401)
        request('/api/items', extra={'Authorization': 'Bearer wrong'}, expected=401)
        request('/api/items', extra={'Origin': 'https://other.invalid'}, expected=403)
        request('/api/items', extra={'Host': 'other.invalid'}, expected=403)
        _, response_headers = request('/?key=' + key, authorized=False, expected=303)
        assert response_headers['location'] == '/'
        cookie = response_headers['set-cookie'].split(';', 1)[0]
        assert 'HttpOnly' in response_headers['set-cookie'] and 'SameSite=Strict' in response_headers['set-cookie']
        page, _ = request('/', extra={'Cookie': cookie}, authorized=False)
        assert b'<html' in page
        for asset in ['app.js', 'app-core.js', 'app-utils.js', 'i18n.js', 'styles.css']:
            request('/' + asset, extra={'Cookie': cookie}, authorized=False)
        request('/api/admin/items', expected=404)
        rpc('start', config, error=True)
        assert rpc('add_files', [str(original)]) == 1
        snapshot = rpc('snapshot')
        item = snapshot['items'][0]
        path = '/api/items/' + item['id'] + '/download'
        data, h = request(path, extra={'Range': 'bytes=3-6'}, expected=206)
        assert data == b'3456' and h['content-range'] == 'bytes 3-6/16'
        data, h = request(path, method='HEAD')
        assert not data and int(h['content-length']) == 16
        request(path, extra={'Range': 'bytes=99-'}, expected=416)
        assert rpc('item_path', item['id']) == str(original)
        save = work / 'saved.txt'
        rpc('save_as', {'id': item['id'], 'path': str(save)})
        assert save.read_bytes() == original.read_bytes()
        save.write_bytes(b'keep me')
        rpc('save_as', {'id': item['id'], 'path': str(save)}, error=True)
        assert save.read_bytes() == b'keep me'
        rpc('save_as', {'id': item['id'], 'path': str(save), 'replace': True})
        assert save.read_bytes() == original.read_bytes()
        _, link_headers = request(path + '?key=' + key, authorized=False, expected=303)
        assert link_headers['location'] == path
        # A changed reference must not mix different versions across resumed
        # downloads, even if its new content has the same byte count.
        stat = original.stat()
        original.write_bytes(b'fedcba9876543210')
        import os
        os.utime(original, ns=(stat.st_atime_ns, stat.st_mtime_ns + 2_000_000_000))
        request(path, extra={'Range': 'bytes=3-6'}, expected=400)
        original.write_bytes(b'0123456789abcdef')
        os.utime(original, ns=(stat.st_atime_ns, stat.st_mtime_ns))

        for content in [b'first', b'second']:
            boundary = 'WinOne' + uuid.uuid4().hex
            body = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="same.txt"\r\nContent-Type: text/plain\r\n\r\n'.encode()
                    + content + f'\r\n--{boundary}--\r\n'.encode())
            request('/api/upload', 'POST', body, {'Content-Type': 'multipart/form-data; boundary=' + boundary})
        assert sorted(p.read_bytes() for p in received.iterdir()) == [b'first', b'second']
        request('/api/text', 'POST', json.dumps({'content': '手机文本'}).encode(), {'Content-Type': 'application/json'})
        rpc('add_text', '本机文本')
        assert len(rpc('snapshot')['items']) == 5
        # Empty uploads remain real files; device names cannot become Windows
        # devices, and user-controlled path components never escape received.
        for filename in ['empty.txt', 'CON.txt', 'LPT1', '../escape.txt', 'trailing. ']:
            boundary = 'WinOne' + uuid.uuid4().hex
            body = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{filename}"\r\nContent-Type: application/octet-stream\r\n\r\n'
                    f'\r\n--{boundary}--\r\n').encode()
            request('/api/upload', 'POST', body, {'Content-Type': 'multipart/form-data; boundary=' + boundary})
        uploads = list(received.iterdir())
        assert len(uploads) == 7, uploads
        assert sum(p.stat().st_size == 0 for p in uploads) == 5
        assert not (work / 'escape.txt').exists()
        assert all(p.name.upper().split('.')[0] not in ['CON', 'LPT1'] for p in uploads)
        assert all(p.name == p.name.rstrip('. ') for p in uploads)
        rpc('remove', item['id'])
        assert original.read_bytes() == b'0123456789abcdef'

        # Real SSE must receive initial state and terminate when sharing stops.
        event_conn = http.client.HTTPConnection('127.0.0.1', port, timeout=6)
        event_conn.request('GET', '/api/events', headers=headers)
        events = event_conn.getresponse()
        assert events.status == 200
        assert b'data:' in events.readline()
        rpc('stop')
        assert b'share-stopped' in events.read()
        event_conn.close()
        assert process.wait(timeout=6) == 0
        probe = socket.socket()
        probe.settimeout(1)
        assert probe.connect_ex(('127.0.0.1', port)) != 0
        probe.close()
        assert (work / 'data' / 'items.json').exists()
        print(json.dumps({'passed': True, 'scope': ['auth', 'origin', 'host', 'cookie', 'assets', 'native management', 'range', 'upload collision', 'text', 'SSE', 'shutdown'], 'work': str(work)}, ensure_ascii=False))
    finally:
        if process.poll() is None:
            process.kill()
        process.wait(timeout=8)
        reads.shutdown(wait=True)


if __name__ == '__main__':
    main()
