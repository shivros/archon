package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"text/tabwriter"

	"control/internal/types"
)

const version = "dev"

func printSessions(output io.Writer, sessions []*types.Session) {
	writer := tabwriter.NewWriter(output, 0, 8, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tSTATUS\tPROVIDER\tPID\tTITLE")
	for _, session := range sessions {
		pid := "-"
		if session.PID > 0 {
			pid = fmt.Sprintf("%d", session.PID)
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", session.ID, session.Status, session.Provider, pid, session.Title)
	}
	_ = writer.Flush()
}

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func exitOnErr(label string, err error, stderr io.Writer) {
	if err == nil {
		return
	}
	fmt.Fprintf(stderr, "%s error: %v\n", label, err)
	os.Exit(1)
}

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		var revision string
		var modified string
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value
			}
		}
		if revision != "" {
			if modified == "true" {
				return revision + "-dirty"
			}
			return revision
		}
	}

	exe, err := os.Executable()
	if err == nil {
		file, err := os.Open(exe)
		if err == nil {
			defer file.Close()
			hasher := sha256.New()
			if _, err := io.Copy(hasher, file); err == nil {
				sum := hasher.Sum(nil)
				return fmt.Sprintf("bin-%x", sum[:6])
			}
		}
	}

	return version
}
