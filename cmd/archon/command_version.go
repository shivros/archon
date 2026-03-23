package main

import (
	"flag"
	"fmt"
	"io"
)

type versionFormatter interface {
	Format(buildMetadata) string
}

type textVersionFormatter struct{}

func (textVersionFormatter) Format(metadata buildMetadata) string {
	return fmt.Sprintf(
		"version: %s\ncommit: %s\nbuild_date: %s\n",
		metadata.Version,
		metadata.Commit,
		metadata.BuildDate,
	)
}

type VersionCommand struct {
	stdout           io.Writer
	stderr           io.Writer
	metadataProvider buildMetadataProvider
	formatter        versionFormatter
}

func NewVersionCommand(stdout, stderr io.Writer) *VersionCommand {
	return NewVersionCommandWithDependencies(
		stdout,
		stderr,
		defaultBuildMetadataProvider(),
		textVersionFormatter{},
	)
}

func NewVersionCommandWithDependencies(
	stdout, stderr io.Writer,
	metadataProvider buildMetadataProvider,
	formatter versionFormatter,
) *VersionCommand {
	if metadataProvider == nil {
		metadataProvider = defaultBuildMetadataProvider()
	}
	if formatter == nil {
		formatter = textVersionFormatter{}
	}
	return &VersionCommand{
		stdout:           stdout,
		stderr:           stderr,
		metadataProvider: metadataProvider,
		formatter:        formatter,
	}
}

func (c *VersionCommand) Run(args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	metadata := c.metadataProvider.Snapshot()
	_, err := io.WriteString(c.stdout, c.formatter.Format(metadata))
	return err
}
