//go:build js

package rl

import (
	"syscall/js"
	"unsafe"
)

// Browsers keep audio suspended until the user clicks or presses a key on the
// page; raylib's audio backend resumes it then.

func InitAudioDevice()               { call("InitAudioDevice") }
func CloseAudioDevice()              { call("CloseAudioDevice") }
func IsAudioDeviceReady() bool       { return truthy(call("IsAudioDeviceReady")) }
func SetMasterVolume(volume float32) { call("SetMasterVolume", volume) }

// Sounds and music stay in raylib's heap. Go can't hold C pointers in pointer
// fields, so each gets a Go placeholder in Stream.Buffer that keys the C
// pointer here; copies of a Sound share the key, as they share the buffer in
// raylib. The placeholder has no contents.
var (
	sounds = map[*AudioBuffer]uint32{}
	tracks = map[*AudioBuffer]uint32{}
)

// wAudioInfo matches WAudioInfo in glue.c.
type wAudioInfo struct {
	FrameCount, SampleRate, SampleSize, Channels uint32
}

func streamOf(info wAudioInfo) AudioStream {
	return AudioStream{Buffer: new(AudioBuffer), SampleRate: info.SampleRate, SampleSize: info.SampleSize, Channels: info.Channels}
}

func keepSound(p uint32) Sound {
	if p == 0 {
		return Sound{}
	}
	call("w_SoundInfo", p, out())
	info := result[wAudioInfo]()
	s := Sound{Stream: streamOf(info), FrameCount: info.FrameCount}
	sounds[s.Stream.Buffer] = p
	return s
}

// sound returns the C pointer for s, or 0 for a sound that isn't loaded.
func sound(s Sound) uint32 {
	if s.Stream.Buffer == nil {
		return 0
	}
	return sounds[s.Stream.Buffer]
}

// LoadSound loads from raylib's virtual filesystem: files must be bundled
// with the page (web/build.sh -a).
func LoadSound(fileName string) Sound {
	return keepSound(uint32(call("w_LoadSound", argString(fileName)).Int()))
}

func LoadSoundFromWave(wave Wave) Sound {
	size := int(wave.FrameCount * wave.Channels * wave.SampleSize / 8)
	if wave.Data == nil || size == 0 {
		return Sound{}
	}
	data := malloc(unsafe.Slice((*byte)(wave.Data), size))
	defer free(data)
	return keepSound(uint32(call("w_LoadSoundFromWave", wave.FrameCount, wave.SampleRate, wave.SampleSize, wave.Channels, data).Int()))
}

func LoadSoundAlias(source Sound) Sound {
	p := sound(source)
	if p == 0 {
		return Sound{}
	}
	return keepSound(uint32(call("w_LoadSoundAlias", p).Int()))
}

func IsSoundValid(s Sound) bool { return sound(s) != 0 }

func unloadSound(s Sound, fn string) {
	if p := sound(s); p != 0 {
		delete(sounds, s.Stream.Buffer)
		call(fn, p)
	}
}

func UnloadSound(s Sound)      { unloadSound(s, "w_UnloadSound") }
func UnloadSoundAlias(s Sound) { unloadSound(s, "w_UnloadSoundAlias") }

// soundCall calls fn with the sound's pointer and args, doing nothing for a
// sound that isn't loaded (raylib ignores those too).
func soundCall(fn string, s Sound, args ...any) js.Value {
	p := sound(s)
	if p == 0 {
		return js.ValueOf(0)
	}
	return call(fn, append([]any{p}, args...)...)
}

func PlaySound(s Sound)                      { soundCall("w_PlaySound", s) }
func StopSound(s Sound)                      { soundCall("w_StopSound", s) }
func PauseSound(s Sound)                     { soundCall("w_PauseSound", s) }
func ResumeSound(s Sound)                    { soundCall("w_ResumeSound", s) }
func IsSoundPlaying(s Sound) bool            { return truthy(soundCall("w_IsSoundPlaying", s)) }
func SetSoundVolume(s Sound, volume float32) { soundCall("w_SetSoundVolume", s, volume) }
func SetSoundPitch(s Sound, pitch float32)   { soundCall("w_SetSoundPitch", s, pitch) }
func SetSoundPan(s Sound, pan float32)       { soundCall("w_SetSoundPan", s, pan) }

// Music. Looping is read from the Go value, as raylib reads its own copy.

// LoadMusicStream streams from raylib's virtual filesystem: files must be
// bundled with the page (web/build.sh -a).
func LoadMusicStream(fileName string) Music {
	p := uint32(call("w_LoadMusicStream", argString(fileName)).Int())
	if p == 0 {
		return Music{}
	}
	call("w_MusicInfo", p, out())
	info := result[wAudioInfo]()
	m := Music{Stream: streamOf(info), FrameCount: info.FrameCount, Looping: true}
	tracks[m.Stream.Buffer] = p
	return m
}

func track(m Music) uint32 {
	if m.Stream.Buffer == nil {
		return 0
	}
	return tracks[m.Stream.Buffer]
}

func IsMusicValid(m Music) bool { return track(m) != 0 }

func UnloadMusicStream(m Music) {
	if p := track(m); p != 0 {
		delete(tracks, m.Stream.Buffer)
		call("w_UnloadMusicStream", p)
	}
}

func musicCall(fn string, m Music, args ...any) js.Value {
	p := track(m)
	if p == 0 {
		return js.ValueOf(0)
	}
	return call(fn, append([]any{p}, args...)...)
}

func PlayMusicStream(m Music)                   { musicCall("w_PlayMusicStream", m) }
func StopMusicStream(m Music)                   { musicCall("w_StopMusicStream", m) }
func PauseMusicStream(m Music)                  { musicCall("w_PauseMusicStream", m) }
func ResumeMusicStream(m Music)                 { musicCall("w_ResumeMusicStream", m) }
func IsMusicStreamPlaying(m Music) bool         { return truthy(musicCall("w_IsMusicStreamPlaying", m)) }
func UpdateMusicStream(m Music)                 { musicCall("w_UpdateMusicStream", m, b2i(m.Looping)) }
func SeekMusicStream(m Music, position float32) { musicCall("w_SeekMusicStream", m, position) }
func SetMusicVolume(m Music, volume float32)    { musicCall("w_SetMusicVolume", m, volume) }
func SetMusicPitch(m Music, pitch float32)      { musicCall("w_SetMusicPitch", m, pitch) }
func SetMusicPan(m Music, pan float32)          { musicCall("w_SetMusicPan", m, pan) }
func GetMusicTimeLength(m Music) float32 {
	return float32(musicCall("w_GetMusicTimeLength", m).Float())
}
func GetMusicTimePlayed(m Music) float32 {
	return float32(musicCall("w_GetMusicTimePlayed", m).Float())
}
