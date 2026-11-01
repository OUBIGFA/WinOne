package chat

import (
	"bytes"
	"encoding/binary"
	"math"
	"sync"
	"syscall"
	"unsafe"
)

// chime is the completion sound, as the React page's playChatDone: two
// sine tones (C5 then E5, 0.18 s apart), each fading from 0.18 to 0.001
// over 0.45 s. It is rendered once as a WAV in memory.
var chime = sync.OnceValue(func() []byte {
	const rate = 44100
	samples := make([]float64, int(0.7*rate))
	for _, tone := range []struct{ offset, frequency float64 }{{0, 523.25}, {0.18, 659.25}} {
		start := int(tone.offset * rate)
		for i := 0; i < int(0.5*rate) && start+i < len(samples); i++ {
			t := float64(i) / rate
			// Exponential ramp from 0.18 to 0.001 over 0.45 s, held after.
			gain := 0.18 * math.Pow(0.001/0.18, math.Min(t, 0.45)/0.45)
			samples[start+i] += gain * math.Sin(2*math.Pi*tone.frequency*t)
		}
	}
	var pcm bytes.Buffer
	for _, s := range samples {
		_ = binary.Write(&pcm, binary.LittleEndian, int16(math.Max(-1, math.Min(1, s))*32767))
	}
	var wav bytes.Buffer
	write := func(v any) { _ = binary.Write(&wav, binary.LittleEndian, v) }
	wav.WriteString("RIFF")
	write(uint32(36 + pcm.Len()))
	wav.WriteString("WAVEfmt ")
	write(uint32(16))
	write(uint16(1))        // PCM
	write(uint16(1))        // mono
	write(uint32(rate))     // sample rate
	write(uint32(rate * 2)) // byte rate
	write(uint16(2))        // block align
	write(uint16(16))       // bits per sample
	wav.WriteString("data")
	write(uint32(pcm.Len()))
	wav.Write(pcm.Bytes())
	return wav.Bytes()
})

var playSound = syscall.NewLazyDLL("winmm.dll").NewProc("PlaySoundW")

// playDone plays the chime without waiting. The sound stays in memory
// for the process, as PlaySound's asynchronous memory playback needs.
func playDone() {
	data := chime()
	const (
		async     = 0x0001
		noDefault = 0x0002
		memory    = 0x0004
	)
	_, _, _ = playSound.Call(uintptr(unsafe.Pointer(&data[0])), 0, async|noDefault|memory)
}
