package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"songarooni/internal/config"
	control_panel "songarooni/internal/control-panel"
	"songarooni/internal/pedal"
	"songarooni/internal/slideshow"
	"songarooni/internal/songs"
	"songarooni/internal/state"
	"strings"
	"syscall"

	"golang.org/x/sync/errgroup"
)

func main() {
	cfg, err := getConfig()
	if err != nil {
		panic(err)
	}
	err = logger(cfg)
	if err != nil {
		panic(err)
	}
	log.Printf("loaded config and logger")
	catalog, err := songs.LoadJSON(cfg.SongsFile, cfg.SlideDir, cfg.IncludeEmptySongs)
	if err != nil {
		log.Fatalf("songs.json invalid, refusing to start:\n%v", err)
	}
	log.Printf("loaded %d songs from %s", len(catalog), cfg.SongsFile)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// channels
	pedalChan := make(chan pedal.PedalControls)
	stop := make(chan os.Signal, 1)

	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	controller := state.New(catalog, nil, pedalChan)
	show := slideshow.New(cfg.BrowserSlideshowAddr, cfg.SlideDir, getSongMap(catalog), cfg.LogoFrequency, cfg.CommercialFrequency, cfg.SlideIntervalSeconds, strings.TrimPrefix(cfg.Addr, ":"))

	// goroutines
	g := &errgroup.Group{}

	g.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case pedalControl := <-pedalChan:
				log.Printf("received pedal control: %s", pedalControl)

			}
		}
	})

	g.Go(func() error {
		controller.Watch(ctx)
		return nil
	})

	g.Go(func() error {
		pedal.Watch(ctx, cfg.PedalDeviceName, pedalChan)
		return nil
	})

	g.Go(func() error {
		control_panel.Run(ctx, controller, *cfg)
		return nil
	})

	g.Go(func() error {
		show.Run(ctx)
		return nil
	})

	// shut down gracefully on SIGINT or SIGTERM
	<-stop
	cancel()
	log.Println("shutting down")

	if err := g.Wait(); err != nil {
		log.Fatalf("error: %v", err)
	}

}

/* setup functions */

func getConfig() (*config.Config, error) {
	configPath := flag.String("config", "config.yaml", "path to config.yaml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func logger(cfg *config.Config) error {
	logFile, err := os.Create(cfg.LogFile) // truncates on every startup, see initial-plan.md Phase 6
	if err != nil {
		fmt.Fprintln(os.Stderr, "songarooni: opening log file:", err)
		return err
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return nil
}

func getSongMap(catalog []songs.Song) map[int]string {
	songMap := make(map[int]string, len(catalog))
	for _, s := range catalog {
		songMap[s.ID] = s.Slides
	}
	return songMap
}
