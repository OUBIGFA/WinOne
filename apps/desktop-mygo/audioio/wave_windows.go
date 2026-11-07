// Package audioio owns native Windows waveform devices, never ASR, provider
// credentials, endpointing or task submission. Those remain in Cindy's Host.
// References: Microsoft WinMM waveInOpen/waveOutOpen/waveInAddBuffer docs.
package audioio

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var winmm = windows.NewLazySystemDLL("winmm.dll")
var waveInOpen = winmm.NewProc("waveInOpen")
var waveInPrepare = winmm.NewProc("waveInPrepareHeader")
var waveInAdd = winmm.NewProc("waveInAddBuffer")
var waveInStart = winmm.NewProc("waveInStart")
var waveInReset = winmm.NewProc("waveInReset")
var waveInUnprepare = winmm.NewProc("waveInUnprepareHeader")
var waveInClose = winmm.NewProc("waveInClose")
var waveInCount = winmm.NewProc("waveInGetNumDevs")
var waveInCaps = winmm.NewProc("waveInGetDevCapsW")
var waveOutOpen = winmm.NewProc("waveOutOpen")
var waveOutPrepare = winmm.NewProc("waveOutPrepareHeader")
var waveOutWrite = winmm.NewProc("waveOutWrite")
var waveOutReset = winmm.NewProc("waveOutReset")
var waveOutUnprepare = winmm.NewProc("waveOutUnprepareHeader")
var waveOutClose = winmm.NewProc("waveOutClose")

const mapper = uint32(0xffffffff)
const callbackEvent = 0x00050000
const headerDone = 1

type waveFormat struct {
	Tag, Channels           uint16
	Rate, BytesPerSecond    uint32
	BlockAlign, Bits, Extra uint16
}
type waveHeader struct {
	Data             uintptr
	Length, Recorded uint32
	User             uintptr
	Flags, Loops     uint32
	Next, Reserved   uintptr
}
type inputCaps struct {
	Manufacturer, Product uint16
	Version               uint32
	Name                  [32]uint16
	Formats               uint32
	Channels, Reserved    uint16
}
type InputDevice struct {
	ID, Name string
	index    uint32
}

func Inputs() ([]InputDevice, error) {
	count, _, _ := waveInCount.Call()
	devices := []InputDevice{{ID: "", Name: "Default", index: mapper}}
	for index := uintptr(0); index < count; index++ {
		var caps inputCaps
		code, _, _ := waveInCaps.Call(index, uintptr(unsafe.Pointer(&caps)), unsafe.Sizeof(caps))
		if code != 0 {
			return nil, fmt.Errorf("WinMM device query failed (%d)", code)
		}
		name := windows.UTF16ToString(caps.Name[:])
		id := fmt.Sprintf("wavein:%d:%d:%s", caps.Manufacturer, caps.Product, name)
		devices = append(devices, InputDevice{ID: id, Name: name, index: uint32(index)})
	}
	return devices, nil
}
func format(rate int) waveFormat {
	return waveFormat{Tag: 1, Channels: 1, Rate: uint32(rate), BytesPerSecond: uint32(rate * 2), BlockAlign: 2, Bits: 16}
}
func mmresult(code uintptr, kind string) error {
	if code != 0 {
		return fmt.Errorf("WinMM %s failed (%d)", kind, code)
	}
	return nil
}

type inputBuffer struct {
	data     []byte
	header   waveHeader
	pin      runtime.Pinner
	prepared bool
}
type Input struct {
	cancel  context.CancelFunc
	done    chan struct{}
	ctx     context.Context
	handle  uintptr
	event   windows.Handle
	buffers []*inputBuffer
	send    func([]byte) error
	fail    func(error)
}

var inputs = struct {
	sync.Mutex
	active map[*Input]bool
}{active: map[*Input]bool{}}

// StartInput captures mono PCM16 at the Host's 16 kHz wire rate. No device is
// opened by queries or default tests; the user's explicit control starts it.
func StartInput(ctx context.Context, deviceID string, send func([]byte) error, fail func(error)) (*Input, error) {
	devices, err := Inputs()
	if err != nil {
		return nil, err
	}
	index := mapper
	found := deviceID == ""
	for _, device := range devices {
		if device.ID == deviceID {
			index = device.index
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("selected microphone is unavailable")
	}
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	p := &Input{ctx: ctx, cancel: cancel, done: make(chan struct{}), event: event, send: send, fail: fail}
	wf := format(16000)
	code, _, _ := waveInOpen.Call(uintptr(unsafe.Pointer(&p.handle)), uintptr(index), uintptr(unsafe.Pointer(&wf)), uintptr(event), 0, callbackEvent)
	if err = mmresult(code, "input open"); err != nil {
		cancel()
		windows.CloseHandle(event)
		return nil, err
	}
	for range 6 {
		buffer := &inputBuffer{data: make([]byte, 3200)}
		buffer.header.Data = uintptr(unsafe.Pointer(&buffer.data[0]))
		buffer.header.Length = uint32(len(buffer.data))
		buffer.pin.Pin(&buffer.data[0])
		buffer.pin.Pin(&buffer.header)
		p.buffers = append(p.buffers, buffer)
		code, _, _ = waveInPrepare.Call(p.handle, uintptr(unsafe.Pointer(&buffer.header)), unsafe.Sizeof(buffer.header))
		if err = mmresult(code, "input prepare"); err != nil {
			p.dispose()
			return nil, err
		}
		buffer.prepared = true
		code, _, _ = waveInAdd.Call(p.handle, uintptr(unsafe.Pointer(&buffer.header)), unsafe.Sizeof(buffer.header))
		if err = mmresult(code, "input buffer"); err != nil {
			p.dispose()
			return nil, err
		}
	}
	code, _, _ = waveInStart.Call(p.handle)
	if err = mmresult(code, "input start"); err != nil {
		p.dispose()
		return nil, err
	}
	inputs.Lock()
	inputs.active[p] = true
	inputs.Unlock()
	go p.run()
	return p, nil
}
func (p *Input) Stop()                 { p.cancel() }
func (p *Input) Done() <-chan struct{} { return p.done }
func SuspendAll() {
	inputs.Lock()
	active := make([]*Input, 0, len(inputs.active))
	for p := range inputs.active {
		active = append(active, p)
	}
	inputs.Unlock()
	for _, p := range active {
		p.Stop()
	}
}
func (p *Input) dispose() {
	p.cancel()
	if p.handle != 0 {
		waveInReset.Call(p.handle)
	}
	for _, buffer := range p.buffers {
		if buffer.prepared {
			waveInUnprepare.Call(p.handle, uintptr(unsafe.Pointer(&buffer.header)), unsafe.Sizeof(buffer.header))
		}
		buffer.pin.Unpin()
	}
	if p.handle != 0 {
		waveInClose.Call(p.handle)
		p.handle = 0
	}
	windows.CloseHandle(p.event)
}
func (p *Input) run() {
	defer close(p.done)
	defer func() { inputs.Lock(); delete(inputs.active, p); inputs.Unlock(); p.dispose() }()
	queue := make(chan []byte, 50)
	errors := make(chan error, 1)
	var consumer sync.WaitGroup
	consumer.Add(1)
	go func() {
		defer consumer.Done()
		for {
			select {
			case <-p.ctx.Done():
				return
			case chunk, ok := <-queue:
				if !ok {
					return
				}
				if err := p.send(chunk); err != nil {
					select {
					case errors <- err:
					default:
					}
					p.cancel()
					return
				}
			}
		}
	}()
	defer func() { close(queue); consumer.Wait() }()
	var failure error
	for p.ctx.Err() == nil {
		_, err := windows.WaitForSingleObject(p.event, 100)
		if err != nil {
			failure = err
			break
		}
		for _, buffer := range p.buffers {
			if buffer.header.Flags&headerDone == 0 {
				continue
			}
			n := min(int(buffer.header.Recorded), len(buffer.data))
			chunk := append([]byte(nil), buffer.data[:n]...)
			code, _, _ := waveInAdd.Call(p.handle, uintptr(unsafe.Pointer(&buffer.header)), unsafe.Sizeof(buffer.header))
			if err = mmresult(code, "input buffer"); err != nil {
				failure = err
				break
			}
			if n > 0 {
				select {
				case queue <- chunk:
				default:
					failure = fmt.Errorf("microphone audio queue overflow; speech was not discarded silently")
					p.cancel()
				}
			}
		}
		if failure != nil {
			break
		}
	}
	p.cancel()
	select {
	case err := <-errors:
		failure = err
	default:
	}
	if failure != nil && p.fail != nil {
		p.fail(failure)
	}
}

type Output struct {
	ctx    context.Context
	handle uintptr
	event  windows.Handle
	closed bool
}

func OpenOutput(ctx context.Context, sampleRate int) (*Output, error) {
	if sampleRate < 8000 || sampleRate > 192000 {
		return nil, fmt.Errorf("invalid PCM rate")
	}
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil, err
	}
	p := &Output{ctx: ctx, event: event}
	wf := format(sampleRate)
	code, _, _ := waveOutOpen.Call(uintptr(unsafe.Pointer(&p.handle)), uintptr(mapper), uintptr(unsafe.Pointer(&wf)), uintptr(event), 0, callbackEvent)
	if err = mmresult(code, "output open"); err != nil {
		windows.CloseHandle(event)
		return nil, err
	}
	return p, nil
}

// Write blocks only the owning playback goroutine, never the UI thread. PCM
// stays pinned until WinMM is done or reset has returned its buffer.
func (p *Output) Write(data []byte) error {
	if p.closed {
		return context.Canceled
	}
	if err := p.ctx.Err(); err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	if len(data)%2 != 0 {
		return fmt.Errorf("incomplete PCM sample")
	}
	buffer := append([]byte(nil), data...)
	header := waveHeader{Data: uintptr(unsafe.Pointer(&buffer[0])), Length: uint32(len(buffer))}
	var pin runtime.Pinner
	pin.Pin(&buffer[0])
	pin.Pin(&header)
	defer pin.Unpin()
	code, _, _ := waveOutPrepare.Call(p.handle, uintptr(unsafe.Pointer(&header)), unsafe.Sizeof(header))
	if err := mmresult(code, "output prepare"); err != nil {
		return err
	}
	defer waveOutUnprepare.Call(p.handle, uintptr(unsafe.Pointer(&header)), unsafe.Sizeof(header))
	code, _, _ = waveOutWrite.Call(p.handle, uintptr(unsafe.Pointer(&header)), unsafe.Sizeof(header))
	if err := mmresult(code, "output write"); err != nil {
		return err
	}
	for header.Flags&headerDone == 0 {
		if err := p.ctx.Err(); err != nil {
			waveOutReset.Call(p.handle)
			return err
		}
		if _, err := windows.WaitForSingleObject(p.event, 100); err != nil {
			waveOutReset.Call(p.handle)
			return err
		}
	}
	return nil
}
func (p *Output) Close() {
	if p.closed {
		return
	}
	p.closed = true
	waveOutReset.Call(p.handle)
	waveOutClose.Call(p.handle)
	windows.CloseHandle(p.event)
}
