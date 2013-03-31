// Nyfikend is a daemon which checks if pages have been updated and notifies the
// user.
package main

import (
	"encoding/gob"
	"log"
	"math"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/howeyc/fsnotify"
	"github.com/karlek/nyfiken/cli"
	"github.com/karlek/nyfiken/ini"
	"github.com/karlek/nyfiken/page"
	"github.com/karlek/nyfiken/settings"
)

// Error wrapper.
func main() {
	err := nyfikend()
	if err != nil {
		log.Fatalln("nyfikend:", err)
	}
}

var pages []*page.Page

func nyfikend() (err error) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Load uncleared updates from last execution.
	err = loadUpdates()
	if err != nil {
		return err
	}

	pages, err = ini.ReadIni(settings.ConfigPath, settings.PagesPath)
	if err != nil {
		return err
	}

	// Change settings files only when config files are modified.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	go watchConfig(watcher)
	err = watcher.Watch(settings.NyfikenRoot)
	if err != nil {
		log.Fatalln(err)
	}

	// Listen for nyfikenc queries.
	go cli.Listen()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			watcher.Close()

			err = saveUpdates()
			if err != nil {
				log.Fatalln(err)
			}
			os.Exit(1)
		}
	}()

	var secondsElapsed float64
	for ; ; secondsElapsed++ {
		// A channel in which errors are sent from p.Check()
		errChan := make(chan error)

		// The number of checks currently taking place
		var numChecks int
		for _, p := range pages {
			// If the seconds elapsed modulo the duration of the interval in
			// seconds is equal to zero, the page should be checked.
			if math.Mod(float64(secondsElapsed), p.Settings.Interval.Seconds()) != 0 {
				continue
			}
			// Start a go-routine to check if the page has been updated.
			go p.Check(errChan)
			numChecks++
		}

		// For each check that took place, listen if any check returned an error
		go func(ch chan error, nChecks int) {
			for i := 0; i < nChecks; i++ {
				if err := <-ch; err != nil {
					log.Println(err)
				}
			}
		}(errChan, numChecks)

		time.Sleep(1 * time.Second)
	}

	return nil
}

// Reads config files only when they are modified.
func watchConfig(watcher *fsnotify.Watcher) {
	var err error
	for {
		select {
		case ev := <-watcher.Event:
			if ev != nil {
				if ev.Name == settings.PagesPath {
					// Retrieve an array of pages from INI file.
					pages, err = ini.ReadPages(settings.PagesPath)
				} else if ev.Name == settings.ConfigPath {
					// Read settings from config file.
					err = ini.ReadSettings(settings.ConfigPath)
				}
			}
		case err = <-watcher.Error:
			// Will fatal after select statement
		}
		if err != nil {
			log.Fatalln("error:", err)
		}
	}
}

// Saves uncleared updates for next execution.
func saveUpdates() (err error) {
	f, err := os.Create(settings.UpdatesPath)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := gob.NewEncoder(f)

	err = enc.Encode(&settings.Updates)
	if err != nil {
		return err
	}
	return nil
}

// Retrieves uncleared updates from last execution.
func loadUpdates() (err error) {
	f, err := os.Open(settings.UpdatesPath)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := gob.NewDecoder(f)

	err = dec.Decode(&settings.Updates)
	if err != nil {
		return err
	}
	return nil
}
