package randomizer

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type StdinPrompter struct {
	reader *bufio.Reader
}

func NewStdinPrompter() *StdinPrompter {
	return &StdinPrompter{
		reader: bufio.NewReader(os.Stdin),
	}
}

func (p *StdinPrompter) Prompt(message string) (string, error) {
	fmt.Print(message + " ")
	input, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

func (p *StdinPrompter) PromptWithDefault(message, defaultValue string) (string, error) {
	fmt.Printf("%s [%s]: ", message, defaultValue)
	input, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}
	return input, nil
}

func (p *StdinPrompter) ChooseFromList(message string, options []string) (string, error) {
	fmt.Println(message + ":")
	for i, opt := range options {
		fmt.Printf("  %d: %s\n", i+1, opt)
	}
	fmt.Print("Enter number: ")

	input, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	var index int
	if _, err := fmt.Sscanf(strings.TrimSpace(input), "%d", &index); err != nil || index < 1 || index > len(options) {
		return "", fmt.Errorf("invalid selection")
	}

	return options[index-1], nil
}
