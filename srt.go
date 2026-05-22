package main

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Subtitle struct {
	Index int
	Start time.Duration
	End   time.Duration
	Text  string
}

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

func ParseSRT(r io.Reader) ([]Subtitle, error) {
	scanner := bufio.NewScanner(r)
	var subs []Subtitle
	var cur Subtitle
	state := "index"

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		switch state {
		case "index":
			if strings.TrimSpace(line) == "" {
				continue
			}
			idx, err := strconv.Atoi(strings.TrimSpace(line))
			if err != nil {
				return nil, fmt.Errorf("expected subtitle index, got %q", line)
			}
			cur = Subtitle{Index: idx}
			state = "timestamp"

		case "timestamp":
			start, end, err := parseTimestampLine(line)
			if err != nil {
				return nil, err
			}
			cur.Start = start
			cur.End = end
			state = "text"

		case "text":
			if strings.TrimSpace(line) == "" {
				if cur.Text != "" {
					subs = append(subs, cur)
				}
				cur = Subtitle{}
				state = "index"
			} else {
				cleaned := htmlTagRe.ReplaceAllString(line, "")
				if cur.Text != "" {
					cur.Text += "\n"
				}
				cur.Text += cleaned
			}
		}
	}

	if cur.Text != "" {
		subs = append(subs, cur)
	}

	return subs, scanner.Err()
}

func parseTimestampLine(line string) (start, end time.Duration, err error) {
	parts := strings.SplitN(line, " --> ", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid timestamp line: %q", line)
	}
	start, err = parseSRTTime(strings.TrimSpace(parts[0]))
	if err != nil {
		return
	}
	end, err = parseSRTTime(strings.TrimSpace(parts[1]))
	return
}

func parseSRTTime(s string) (time.Duration, error) {
	// HH:MM:SS,mmm or HH:MM:SS.mmm
	s = strings.ReplaceAll(s, ",", ".")
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid SRT time: %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid hours in %q", s)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid minutes in %q", s)
	}
	sec, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid seconds in %q", s)
	}
	return time.Duration(h)*time.Hour +
		time.Duration(m)*time.Minute +
		time.Duration(sec*float64(time.Second)), nil
}
