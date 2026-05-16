package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/randomizer"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/youtube"
)

const version = "0.5.2"

func main() {
	chunks := flag.Int("n", 190, "Number of update requests per 24 hours")
	input := flag.String("i", "client_secret.json", "Client secret JSON file")
	showVersion := flag.Bool("version", false, "Print version")

	flag.Parse()

	if *showVersion {
		fmt.Printf("youtube-playlist-randomizer version %s\n", version)
		os.Exit(0)
	}

	ctx := context.Background()

	client, err := youtube.NewClient(ctx, *input)
	if err != nil {
		log.Fatalf("Failed to create YouTube client: %v", err)
	}

	prompter := randomizer.NewStdinPrompter()
	rand := randomizer.New(client, *chunks, prompter)

	if err := rand.Run(ctx); err != nil {
		log.Fatalf("Failed to randomize playlist: %v", err)
	}
}
