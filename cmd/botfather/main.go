// Command botfather draws the pictures the bot's owner uploads through
// BotFather by hand, since the Bot API cannot set them: the profile photo and
// the picture over the description in an empty chat.
//
//	go run ./cmd/botfather            # writes docs/botfather/*.png
//	go run ./cmd/botfather -out /tmp  # anywhere else
//
// They are drawn by pkg/card from the road of assets/icon.svg and live outside
// assets/, so the server binary does not embed them.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"gorun/pkg/card"
)

func main() {
	out := flag.String("out", filepath.Join("docs", "botfather"), "directory to write the pictures to")
	flag.Parse()

	if err := write(*out); err != nil {
		fmt.Fprintln(os.Stderr, "botfather:", err)
		os.Exit(1)
	}
}

func write(dir string) error {
	pictures := []struct {
		name string
		draw func() ([]byte, error)
	}{
		{name: "avatar.png", draw: card.Avatar},
		{name: "description.png", draw: card.Description},
	}

	for _, picture := range pictures {
		data, err := picture.draw()
		if err != nil {
			return fmt.Errorf("draw %s: %w", picture.name, err)
		}

		path := filepath.Join(dir, picture.name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Println("wrote", path)
	}

	return nil
}
