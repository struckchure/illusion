// Package audio plays sound effects and music with raylib.
//
// Load files with asset.Loader[audio.Sound] (short effects, kept in memory)
// or asset.Loader[audio.Music] (long tracks, streamed), then play them
// through the [Audio] system parameter.
package audio

import (
	"errors"
	"math"
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"
	"github.com/struckchure/illusion"
	"github.com/struckchure/illusion/asset"
)

// maxVoices is how many copies of one Sound can play at once.
const maxVoices = 8

// Sound is a short effect kept in memory (.wav, .ogg, .mp3, .flac, .qoa).
type Sound struct {
	rl.Sound
	// voices are aliases sharing the sound's data, so it can overlap itself.
	voices []rl.Sound
}

// Music is a long track streamed from disk (.ogg, .mp3, .wav, .flac, .qoa,
// .xm, .mod).
type Music struct {
	rl.Music
}

// Settings is a resource with global audio settings.
type Settings struct {
	// Volume is the master volume, 0..1.
	Volume float32
}

// Playback adjusts one play of a sound. The zero value plays it as is.
type Playback struct {
	// Volume scales the sound, 0..1; 0 means 1.
	Volume float32
	// Pitch scales the speed and pitch; 0 means 1.
	Pitch float32
	// Pan places the sound from -1 (left) to 1 (right).
	Pan float32
}

// Plugin opens the audio device and plays sounds and music.
type Plugin struct{}

// Build implements [illusion.Plugin].
func (Plugin) Build(app *illusion.App) {
	st := &state{music: map[asset.Handle[Music]]struct{}{}, volume: -1}
	app.InsertResource(illusion.R(st))
	app.InitResource(illusion.R(&Settings{Volume: 1}))
	asset.RegisterLoader(app, loadSound, unloadSound)
	asset.RegisterLoader(app, loadMusic, unloadMusic)
	app.AddSystems(illusion.PreStartup, illusion.Fn0(openDevice).Named("audio.open"))
	app.AddSystems(illusion.PostUpdate, illusion.Fn3(update).Named("audio.update"))
}

// Cleanup implements the optional plugin cleanup hook. Sounds and music are
// released by their asset stores before this runs.
func (Plugin) Cleanup(*illusion.App) {
	if rl.IsAudioDeviceReady() {
		rl.CloseAudioDevice()
	}
}

// state tracks which tracks are playing, for streaming.
type state struct {
	music  map[asset.Handle[Music]]struct{} // playing tracks; looping is m.Looping
	volume float32
}

func openDevice() {
	if !rl.IsAudioDeviceReady() {
		rl.InitAudioDevice()
	}
}

// update feeds streaming music and applies the master volume.
func update(st *illusion.Res[state], settings *illusion.Res[Settings], tracks *illusion.Res[asset.Assets[Music]]) {
	s := st.Get()
	if v := settings.Get().Volume; v != s.volume {
		rl.SetMasterVolume(v)
		s.volume = v
	}
	store := tracks.Get()
	for h := range s.music {
		m := store.Get(h)
		if m == nil {
			delete(s.music, h)
			continue
		}
		rl.UpdateMusicStream(m.Music)
		if !rl.IsMusicStreamPlaying(m.Music) {
			delete(s.music, h)
		}
	}
}

func loadSound(path string) (Sound, error) {
	s := rl.LoadSound(path)
	if !rl.IsSoundValid(s) {
		return Sound{}, errors.New("raylib could not load the sound")
	}
	return Sound{Sound: s}, nil
}

func unloadSound(s *Sound) {
	for _, v := range s.voices {
		rl.UnloadSoundAlias(v)
	}
	rl.UnloadSound(s.Sound)
}

func loadMusic(path string) (Music, error) {
	m := rl.LoadMusicStream(path)
	if !rl.IsMusicValid(m) {
		return Music{}, errors.New("raylib could not load the music")
	}
	return Music{Music: m}, nil
}

func unloadMusic(m *Music) {
	rl.StopMusicStream(m.Music)
	rl.UnloadMusicStream(m.Music)
}

// SoundFromSamples makes a sound from mono samples in -1..1, e.g. from
// [Tone]. Store the result in asset.Assets[Sound]. The audio device must be
// open, so call it from Startup or later.
func SoundFromSamples(samples []float32, sampleRate int) Sound {
	if len(samples) == 0 {
		return Sound{}
	}
	wave := rl.Wave{
		FrameCount: uint32(len(samples)),
		SampleRate: uint32(sampleRate),
		SampleSize: 32,
		Channels:   1,
		Data:       unsafe.Pointer(&samples[0]),
	}
	return Sound{Sound: rl.LoadSoundFromWave(wave)} // copies the samples
}

// SampleRate is the rate Tone generates at.
const SampleRate = 44100

// Tone generates a sine beep at freq Hz lasting seconds, with a short attack
// and a fade out so it doesn't click. Pass it to SoundFromSamples with
// SampleRate.
func Tone(freq, seconds float32) []float32 {
	n := int(seconds * SampleRate)
	out := make([]float32, n)
	attack := min(n/20, SampleRate/200)
	for i := range out {
		t := float64(i) / SampleRate
		env := 1 - float64(i)/float64(n)
		if i < attack {
			env *= float64(i) / float64(attack)
		}
		out[i] = float32(math.Sin(2*math.Pi*float64(freq)*t) * env * 0.5)
	}
	return out
}

// Audio is a system parameter for playing sounds and music.
type Audio struct {
	state  *state
	sounds *asset.Assets[Sound]
	tracks *asset.Assets[Music]
}

// InitParam implements [illusion.Param].
func (a *Audio) InitParam(w *ecs.World) {
	a.state = ecs.GetResource[state](w)
	if a.state == nil {
		panic("audio: Audio used without audio.Plugin")
	}
	a.sounds = ecs.GetResource[asset.Assets[Sound]](w)
	a.tracks = ecs.GetResource[asset.Assets[Music]](w)
}

// Play plays a sound once. Playing a sound that's already playing overlaps
// it, up to 8 copies at once.
func (a *Audio) Play(h asset.Handle[Sound]) {
	a.PlayWith(h, Playback{})
}

// PlayWith plays a sound once with adjusted volume, pitch or pan.
func (a *Audio) PlayWith(h asset.Handle[Sound], p Playback) {
	s := a.sounds.Get(h)
	if s == nil || !rl.IsAudioDeviceReady() {
		return
	}
	voice := s.freeVoice()
	rl.SetSoundVolume(voice, orOne(p.Volume))
	rl.SetSoundPitch(voice, orOne(p.Pitch))
	rl.SetSoundPan(voice, max(-1, min(1, p.Pan)))
	rl.PlaySound(voice)
}

// freeVoice returns a copy of the sound that isn't playing, making one if
// there's room, or restarts the original.
func (s *Sound) freeVoice() rl.Sound {
	if !rl.IsSoundPlaying(s.Sound) {
		return s.Sound
	}
	for _, v := range s.voices {
		if !rl.IsSoundPlaying(v) {
			return v
		}
	}
	if len(s.voices) < maxVoices-1 {
		v := rl.LoadSoundAlias(s.Sound)
		s.voices = append(s.voices, v)
		return v
	}
	return s.Sound
}

// PlayMusic starts a track from the beginning, looping it if loop is set.
func (a *Audio) PlayMusic(h asset.Handle[Music], loop bool) {
	m := a.tracks.Get(h)
	if m == nil || !rl.IsAudioDeviceReady() {
		return
	}
	m.Looping = loop
	rl.StopMusicStream(m.Music)
	rl.PlayMusicStream(m.Music)
	a.state.music[h] = struct{}{}
}

// StopMusic stops a track.
func (a *Audio) StopMusic(h asset.Handle[Music]) {
	if m := a.tracks.Get(h); m != nil {
		rl.StopMusicStream(m.Music)
	}
	delete(a.state.music, h)
}

// SetMusicVolume sets a track's volume, 0..1.
func (a *Audio) SetMusicVolume(h asset.Handle[Music], volume float32) {
	if m := a.tracks.Get(h); m != nil {
		rl.SetMusicVolume(m.Music, volume)
	}
}

// SetMusicPitch sets a track's speed and pitch; 1 plays it as is. A looping
// engine or machine can follow its speed with it.
func (a *Audio) SetMusicPitch(h asset.Handle[Music], pitch float32) {
	if m := a.tracks.Get(h); m != nil {
		rl.SetMusicPitch(m.Music, orOne(pitch))
	}
}

// SetMusicPan places a track from -1 (left) to 1 (right).
func (a *Audio) SetMusicPan(h asset.Handle[Music], pan float32) {
	if m := a.tracks.Get(h); m != nil {
		rl.SetMusicPan(m.Music, max(-1, min(1, pan)))
	}
}

// MusicPlaying reports whether a track is playing.
func (a *Audio) MusicPlaying(h asset.Handle[Music]) bool {
	_, ok := a.state.music[h]
	return ok
}

func orOne(v float32) float32 {
	if v == 0 {
		return 1
	}
	return v
}
