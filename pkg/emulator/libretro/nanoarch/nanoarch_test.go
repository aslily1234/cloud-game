package nanoarch

import (
	"crypto/md5"
	"fmt"
	"image"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/giongto35/cloud-game/pkg/config"
)

// Emulator data mock.
type EmulatorMock struct {
	// Libretro compiled lib core name
	core string

	// hardcoded emulator channels
	image chan GameFrame
	audio chan []int16
	input chan InputEvent

	// selected emulator core meta
	meta config.EmulatorMeta

	// shared core paths (can't be changed)
	paths EmulatorPaths
}

// Defines various emulator file paths.
type EmulatorPaths struct {
	assets string
	cores  string
	games  string
}

// Returns a properly stubbed emulator instance.
// Be aware that it extensively uses global variables and
// it should exist in one instance per a test run.
// Don't forget to add at least one image channel consumer
// since it will lock your thread otherwise.
// Don't forget to close emulator mock with shutdownEmulator().
func GetEmulatorMock(room string, system string) *EmulatorMock {
	assetsPath := getAssetsPath()
	metadata := config.EmulatorConfig[system]

	// an emu
	emu := &EmulatorMock{
		core: path.Base(metadata.Path),

		image: make(chan GameFrame, 30),
		audio: make(chan []int16, 30),
		input: make(chan InputEvent, 100),

		meta: metadata,

		paths: EmulatorPaths{
			assets: cleanPath(assetsPath),
			cores:  cleanPath(assetsPath + "emulator/libretro/cores/"),
			games:  cleanPath(assetsPath + "games/"),
		},
	}

	// global video output canvas
	outputImg = image.NewRGBA(image.Rect(0, 0, emu.meta.Width, emu.meta.Height))

	// global emulator instance
	NAEmulator = &naEmulator{
		meta:           emu.meta,
		imageChannel:   emu.image,
		audioChannel:   emu.audio,
		inputChannel:   emu.input,
		controllersMap: map[string][]controllerState{},
		roomID:         room,
		done:           make(chan struct{}, 1),
		lock:           &sync.Mutex{},
	}

	return emu
}

// Returns initialized emulator mock with default params.
// Spawns audio/image channels consumers.
// Don't forget to close emulator mock with shutdownEmulator().
func GetDefaultEmulatorMock(room string, system string, rom string) *EmulatorMock {
	mock := GetEmulatorMock(room, system)
	mock.loadRom(rom)
	go mock.handleVideo(func(_ GameFrame) {})
	go mock.handleAudio(func(_ []int16) {})

	return mock
}

// Load a rom into the emulator.
// The rom will be loaded from emulators' games path.
func (emu EmulatorMock) loadRom(game string) {
	fmt.Printf("%v %v\n", emu.paths.cores, emu.core)
	coreLoad(emu.paths.cores+emu.core, false, false, "")
	coreLoadGame(emu.paths.games + game)
}

// Close the emulator and cleanup its resources.
func (emu EmulatorMock) shutdownEmulator() {
	_ = os.Remove(getLatestSave())

	close(emu.image)
	close(emu.audio)
	close(emu.input)

	nanoarchShutdown()
}

// Emulate one frame with exclusive lock.
func (emu EmulatorMock) emulateOneFrame() {
	NAEmulator.GetLock()
	nanoarchRun()
	NAEmulator.ReleaseLock()
}

// Who needs generics anyway?
// Custom handler for the video channel.
func (emu EmulatorMock) handleVideo(handler func(image GameFrame)) {
	for frame := range emu.image {
		handler(frame)
	}
}

// Custom handler for the audio channel.
func (emu EmulatorMock) handleAudio(handler func(sample []int16)) {
	for frame := range emu.audio {
		handler(frame)
	}
}

// Custom handler for the input channel.
func (emu EmulatorMock) handleInput(handler func(event InputEvent)) {
	for event := range emu.input {
		handler(event)
	}
}

// Returns the current emulator state and
// the latest saved state for its session.
// Locks the emulator.
func (emu EmulatorMock) dumpState() (string, string) {
	NAEmulator.GetLock()

	state, _ := getState()
	stateHash := getHash(state)
	persistedStateHash := getSaveHash()

	fmt.Printf("mem: %v, dat: %v\n", stateHash, persistedStateHash)

	NAEmulator.ReleaseLock()
	return stateHash, persistedStateHash
}

// Returns absolute path to the assets directory.
func getAssetsPath() string {
	appName := "cloud-game"
	var (
		// get app path at runtime
		_, b, _, _ = runtime.Caller(0)
		basePath   = filepath.Dir(strings.SplitAfter(b, appName)[0]) + "/" + appName + "/assets/"
	)

	return basePath
}

// Returns the full path to the emulator latest save.
func getLatestSave() string {
	return cleanPath(NAEmulator.GetHashPath())
}

// Returns latest save hash.
func getSaveHash() string {
	bytes, _ := ioutil.ReadFile(getLatestSave())
	return getHash(bytes)
}

// Returns MD5 hash.
func getHash(bytes []byte) string {
	return fmt.Sprintf("%x", md5.Sum(bytes))
}

// Returns a proper file path for current OS.
func cleanPath(path string) string {
	return filepath.FromSlash(path)
}
