package main

import (
	"testing"

	"github.com/opskat/opskat/internal/app"
	"github.com/opskat/opskat/internal/bootstrap"
)

func TestInitialWindowSizeUsesSavedSizeWithMinimumFallbacks(t *testing.T) {
	t.Parallel()

	width, height := initialWindowSize(&bootstrap.AppConfig{
		WindowWidth:  minWindowWidth + 120,
		WindowHeight: minWindowHeight + 80,
	})
	if width != minWindowWidth+120 {
		t.Fatalf("width = %d, want %d", width, minWindowWidth+120)
	}
	if height != minWindowHeight+80 {
		t.Fatalf("height = %d, want %d", height, minWindowHeight+80)
	}

	width, height = initialWindowSize(&bootstrap.AppConfig{
		WindowWidth:  minWindowWidth - 1,
		WindowHeight: minWindowHeight - 1,
	})
	if width != defaultWindowWidth {
		t.Fatalf("width below minimum = %d, want default %d", width, defaultWindowWidth)
	}
	if height != defaultWindowHeight {
		t.Fatalf("height below minimum = %d, want default %d", height, defaultWindowHeight)
	}
}

func TestNewAppOptionsUsesSavedSizeWithoutLateRecentering(t *testing.T) {
	t.Parallel()

	opts := newAppOptions(&app.App{}, minWindowWidth+200, minWindowHeight+100)
	if opts.Width != minWindowWidth+200 {
		t.Fatalf("Width = %d, want %d", opts.Width, minWindowWidth+200)
	}
	if opts.Height != minWindowHeight+100 {
		t.Fatalf("Height = %d, want %d", opts.Height, minWindowHeight+100)
	}
	if opts.MinWidth != minWindowWidth {
		t.Fatalf("MinWidth = %d, want %d", opts.MinWidth, minWindowWidth)
	}
	if opts.MinHeight != minWindowHeight {
		t.Fatalf("MinHeight = %d, want %d", opts.MinHeight, minWindowHeight)
	}
	if opts.OnDomReady != nil {
		t.Fatal("OnDomReady should be nil so startup centering happens only through Wails' initial window placement")
	}
}
