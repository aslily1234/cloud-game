package nanoarch

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

type testRun struct {
	room           string
	system         string
	rom            string
	emulationTicks int
}

// Tests a successful emulator state save.
func TestSave(t *testing.T) {
	tests := []testRun{
		{
			room:           "test_save_ok_00",
			system:         "gba",
			rom:            "Sushi The Cat.gba",
			emulationTicks: 100,
		},
		{
			room:           "test_save_ok_01",
			system:         "gba",
			rom:            "anguna.gba",
			emulationTicks: 10,
		},
	}

	for _, test := range tests {
		t.Logf("Testing [%v] save with [%v]\n", test.system, test.rom)

		mock := GetDefaultEmulatorMock(test.room, test.system, test.rom)

		for test.emulationTicks > 0 {
			mock.emulateOneFrame()
			test.emulationTicks--
		}

		fmt.Printf("[%-14v] ", "before save")
		snapshot1, _ := mock.dumpState()
		if err := NAEmulator.Save(); err != nil {
			t.Errorf("Save fail %v", err)
		}
		fmt.Printf("[%-14v] ", "after  save")
		snapshot1, snapshot2 := mock.dumpState()

		if snapshot1 != snapshot2 {
			t.Errorf("It seems rom state save has failed: %v != %v", snapshot1, snapshot2)
		}

		mock.shutdownEmulator()
	}
}

// Tests save and restore function:
//
// Emulate n ticks.
// Call save (a).
// Emulate n ticks again.
// Call load from the save (b).
// Compare states (a) and (b), should be =.
//
func TestLoad(t *testing.T) {
	tests := []testRun{
		{
			room:           "test_load_00",
			system:         "nes",
			rom:            "Super Mario Bros.nes",
			emulationTicks: 100,
		},
		{
			room:           "test_load_01",
			system:         "gba",
			rom:            "Sushi The Cat.gba",
			emulationTicks: 1000,
		},
		{
			room:           "test_load_02",
			system:         "gba",
			rom:            "anguna.gba",
			emulationTicks: 100,
		},
	}

	for _, test := range tests {
		t.Logf("Testing [%v] load with [%v]\n", test.system, test.rom)

		mock := GetDefaultEmulatorMock(test.room, test.system, test.rom)

		fmt.Printf("[%-14v] ", "initial")
		mock.dumpState()

		for ticks := test.emulationTicks; ticks > 0; ticks-- {
			mock.emulateOneFrame()
		}
		fmt.Printf("[%-14v] ", fmt.Sprintf("emulated %d", test.emulationTicks))
		mock.dumpState()

		if err := NAEmulator.Save(); err != nil {
			t.Errorf("Save fail %v", err)
		}
		fmt.Printf("[%-14v] ", "saved")
		snapshot1, _ := mock.dumpState()

		for ticks := test.emulationTicks; ticks > 0; ticks-- {
			mock.emulateOneFrame()
		}
		fmt.Printf("[%-14v] ", fmt.Sprintf("emulated %d", test.emulationTicks))
		mock.dumpState()

		if err := NAEmulator.Load(); err != nil {
			t.Errorf("Load fail %v", err)
		}
		fmt.Printf("[%-14v] ", "restored")
		snapshot2, _ := mock.dumpState()

		if snapshot1 != snapshot2 {
			t.Errorf("It seems rom state restore has failed: %v != %v", snapshot1, snapshot2)
		}

		mock.shutdownEmulator()
	}
}

func TestStateConcurrency(t *testing.T) {
	tests := []struct {
		run testRun
		// emulation frame cap
		fps int
		// determine random
		seed int
	}{
		{
			run: testRun{
				room:           "test_concurrency_00",
				system:         "gba",
				rom:            "Sushi The Cat.gba",
				emulationTicks: 120,
			},
			fps:  60,
			seed: 42,
		},
		{
			run: testRun{
				room:           "test_concurrency_01",
				system:         "gba",
				rom:            "anguna.gba",
				emulationTicks: 300,
			},
			fps:  60,
			seed: 42 + 42,
		},
	}

	for _, test := range tests {
		t.Logf("Testing [%v] concurrency with [%v]\n", test.run.system, test.run.rom)

		mock := GetEmulatorMock(test.run.room, test.run.system)
		ops := &sync.WaitGroup{}
		// quantum lock
		qLock := &sync.Mutex{}
		op := 0

		mock.loadRom(test.run.rom)
		go mock.handleVideo(func(frame GameFrame) {
			if len(frame.Image.Pix) == 0 {
				t.Errorf("It seems that rom video frame was empty, which is strange!")
			}
		})
		go mock.handleAudio(func(_ []int16) {})
		go mock.handleInput(func(_ InputEvent) {})

		rand.Seed(int64(test.seed))
		t.Logf("'Random' seed is [%v]\n", test.seed)
		t.Logf("Save path is [%v]\n", getLatestSave())

		//start := time.Now()
		//elapsed := time.Since(start)
		//t.Logf("Emulation of %v ticks has took %.2fs with %.2ffps\n",
		//	test.run.emulationTicks, elapsed.Seconds(), float64(test.run.emulationTicks)/elapsed.Seconds())

		_ = NAEmulator.Save()

		// 60 fps emulation cap
		ticker := time.NewTicker(time.Second / time.Duration(test.fps))

		for range ticker.C {
			select {
			case <-NAEmulator.done:
				mock.shutdownEmulator()
				return
			default:
			}

			op++
			if op > test.run.emulationTicks {
				NAEmulator.Close()
			}

			qLock.Lock()
			mock.emulateOneFrame()
			qLock.Unlock()

			if lucky() && !lucky() {
				ops.Add(1)
				go func() {
					qLock.Lock()
					defer qLock.Unlock()

					mock.dumpState()
					// remove save to reproduce the bug
					_ = NAEmulator.Save()
					_, snapshot1 := mock.dumpState()
					_ = NAEmulator.Load()
					snapshot2, _ := mock.dumpState()

					// Bug or feature?
					// When you load a state from the file
					// without immediate preceding save,
					// it won't be in the loaded state
					// even without calling retro_run.
					// But if you pause the threads with a debugger
					// and execute everything with steps, then it works.
					// Possible background emulation?

					if snapshot1 != snapshot2 {
						t.Errorf("States are inconsistent %v != %v on tick %v\n", snapshot1, snapshot2, op)
					}
					ops.Done()
				}()
			}
		}

		ops.Wait()
	}
}

// Returns random boolean.
func lucky() bool {
	return rand.Intn(2) == 1
}
