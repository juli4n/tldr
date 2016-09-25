package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/fatih/color"
)

const pageURLPattern = "https://raw.githubusercontent.com/tldr-pages/tldr/master/pages/%s/%s.md"

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: tldr <command name>\n")
	}

	command := os.Args[1]
	page, err := fetchPage(getPlatform(), command)

	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}

	// Command found, render and print to stdout.
	fmt.Print(renderPage(page))
}

func fetchPage(platform, command string) (string, error) {
	pages := make(chan *string)

	// Fetch the command from platform and common directories in parallel.
	// Then discard the one that fails.
	go fetch(fmt.Sprintf(pageURLPattern, "common", command), pages)
	go fetch(fmt.Sprintf(pageURLPattern, platform, command), pages)

	for i := 0; i < 2; i++ {
		page := <-pages
		if page != nil {
			return *page, nil
		}
	}
	return "", fmt.Errorf("Command not found: %s", command)
}

func fetch(url string, result chan *string) {
	resp, err := http.Get(url)
	if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err == nil {
			bodyAsString := string(body)
			result <- &bodyAsString
		} else {
			result <- nil
		}
	} else {
		result <- nil
	}
}

func getPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "osx"
	default:
		return runtime.GOOS
	}
}

// Renders simplified page Markdown documentation.
// See https://github.com/rprieto/tldr/blob/master/CONTRIBUTING.md#markdown-format
func renderPage(page string) string {
	result := ""
	s := bufio.NewScanner(strings.NewReader(page))

	for s.Scan() {
		l := s.Text()
		if len(l) == 0 {
			result += "\n"
			continue
		}
		switch l[0] {
		case '#':
			result += "  " + color.MagentaString(strings.TrimLeft(l, "# ")) + "\n"
		case '>':
			result += "  " + strings.TrimLeft(l, "> ") + "\n"
		case '-':
			result += color.GreenString(l) + "\n"
		case '`':
			result += "  " + strings.Trim(l, "`") + "\n"
		default:
			result += l + "\n"
		}
	}
	return result
}
