package main

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/peterbourgon/diskv"
)

const pageURLPattern = "https://raw.githubusercontent.com/tldr-pages/tldr/master/pages/%s/%s.md"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprint(os.Stderr, "What tlrd page do you want?\n")
		os.Exit(-1)
	}

	var content string
	var err error

	page := os.Args[1]

	if content, err = fetchPage(getPlatform(), page); err != nil {
		fmt.Fprintf(os.Stderr, "Page not found: %s\n", page)
		os.Exit(-1)
	}

	// Render and print.
	fmt.Print(renderPage(content))
}

func fetchPage(platform, page string) (string, error) {
	responses := make(chan *string)

	// Fetch the page from platform and common directories in parallel.
	// Then discard the one that fails.
	go fetch(fmt.Sprintf(pageURLPattern, "common", page), responses)
	go fetch(fmt.Sprintf(pageURLPattern, platform, page), responses)

	cache := newCache()
	if content, err := cache.read(page); err == nil {
		return content, nil
	}

	for i := 0; i < 2; i++ {
		content := <-responses
		if content != nil {
			// Page found, write it to the cache.
			cache.write(page, *content)
			return *content, nil
		}
	}
	return "", fmt.Errorf("Page not found: %s", page)
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
func renderPage(content string) string {
	result := ""
	s := bufio.NewScanner(strings.NewReader(content))

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

type pageCache struct {
	diskv *diskv.Diskv
}

// CachedPage is a page stored in the diskv cache.
type CachedPage struct {
	// The page content.
	Page string
	// Unix time (seconds).
	CreatedAt int64
}

func (c *CachedPage) isValid(now int64) bool {
	return now-c.CreatedAt < 60*60*24 // 1 day.
}

func newCache() *pageCache {
	currentUser, err := user.Current()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not read home directory: %v\n", err)
		os.Exit(-1)
	}

	return &pageCache{
		diskv: diskv.New(diskv.Options{
			BasePath: filepath.Join(currentUser.HomeDir, ".tldr", "cache"),
		})}
}

func (c *pageCache) write(page string, content string) {
	var cacheBuffer bytes.Buffer
	encoder := gob.NewEncoder(&cacheBuffer)
	encoder.Encode(CachedPage{
		CreatedAt: time.Now().Unix(),
		Page:      content,
	})
	c.diskv.Write(page, cacheBuffer.Bytes())
}

func (c *pageCache) read(page string) (string, error) {
	var cachedData []byte
	var err error
	if cachedData, err = c.diskv.Read(page); err != nil {
		return "", errors.New("Error loading from cache")
	}

	decoder := gob.NewDecoder(strings.NewReader(string(cachedData)))
	cp := CachedPage{}
	if err = decoder.Decode(&cp); err != nil {
		// Ups, cached data is corrupted.
		c.diskv.Erase(page)
		fmt.Fprint(os.Stderr, color.RedString(
			fmt.Sprintf("Warning: %s is corrupted.\n", c.diskv.BasePath)))
		return "", errors.New("Not found. Corrupted data.")
	}
	if !cp.isValid(time.Now().Unix()) {
		c.diskv.Erase(page)
		return "", errors.New("Not found. Expired entry.")
	}
	return cp.Page, nil
}
