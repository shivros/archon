package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"control/internal/types"
)

func printSessions(output io.Writer, sessions []*types.Session) {
	writer := tabwriter.NewWriter(output, 0, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "ID\tSTATUS\tPROVIDER\tPID\tTITLE")
	for _, session := range sessions {
		pid := "-"
		if session.PID > 0 {
			pid = fmt.Sprintf("%d", session.PID)
		}
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", session.ID, session.Status, session.Provider, pid, session.Title)
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
	_, _ = fmt.Fprintf(stderr, "%s error: %v\n", label, err)
	os.Exit(1)
}
