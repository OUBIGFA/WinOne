using System.Buffers.Binary;

namespace WinOne.Capture;

// The wire format matches the Go Host: little-endian length followed by UTF-8 JSON.
internal static class PipeProtocol
{
    private const int Maximum = 1024 * 1024;
    public static async Task<byte[]> ReadAsync(Stream stream, CancellationToken cancellation)
    {
        var prefix = new byte[4];
        await stream.ReadExactlyAsync(prefix, cancellation);
        var size = BinaryPrimitives.ReadUInt32LittleEndian(prefix);
        if (size is 0 or > Maximum) throw new InvalidDataException("无效的消息长度");
        var data = new byte[(int)size];
        await stream.ReadExactlyAsync(data, cancellation);
        return data;
    }
    public static async Task WriteAsync(Stream stream, byte[] data, CancellationToken cancellation)
    {
        if (data.Length is 0 or > Maximum) throw new InvalidDataException("无效的消息长度");
        var prefix = new byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(prefix, (uint)data.Length);
        await stream.WriteAsync(prefix, cancellation);
        await stream.WriteAsync(data, cancellation);
    }
}
