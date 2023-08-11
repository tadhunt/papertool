package main

import (
	"fmt"
	"github.com/integrii/flaggy"
	"github.com/tadhunt/papertool"
	"strings"
	"net/url"
	"os"
)

type Cmd struct {
	cmd     *flaggy.Subcommand
	handler func(cmd *Cmd) error
}

var (
	serverURL *url.URL
	quiet     = false
	paperProject   = ""
	paperProjectVersion = ""

)

func main() {
	flaggy.SetName(os.Args[0])
	flaggy.SetDescription("Tool for interacting with the Jenkins API")
	flaggy.DefaultParser.AdditionalHelpPrepend = "https://github.com/tadhunt/papertool"
	flaggy.SetVersion("0.1")

	server := ""
	flaggy.String(&server, "", "server", "[required] URL of Jenkins server to interact with")
	flaggy.Bool(&quiet, "", "quiet", "[optional] don't print extra info")
	flaggy.String(&paperProject, "", "project", "[required] Paper project to fetch data from")
	flaggy.String(&paperProjectVersion, "", "project-version", "[optional] version of the project to fetch data from")

	cmds := []*Cmd{
		newGetCmd(),
		newDownloadCmd(),
	}

	for _, cmd := range cmds {
		flaggy.AttachSubcommand(cmd.cmd, 1)
	}

	flaggy.Parse()

	if server == "" {
		flaggy.DefaultParser.ShowHelpWithMessage("-server is required")
		return
	}

	if paperProject == "" {
		flaggy.DefaultParser.ShowHelpWithMessage("-project is required")
	}

	var err error
	serverURL, err = url.Parse(server)
	if err != nil {
		flaggy.DefaultParser.ShowHelpWithMessage(fmt.Sprintf("parse url: %v", err))
		return
	}

	for _, cmd := range cmds {
		if cmd.cmd.Used {
			err := cmd.handler(cmd)
			if err != nil {
				serr, isSyntaxError := err.(*papertool.MetadataSyntaxError)
				if isSyntaxError {
					os.Stderr.WriteString(serr.Raw)
					fmt.Fprintf(os.Stderr, "offset %v\n", serr.Offset)
					flaggy.DefaultParser.ShowHelpWithMessage(fmt.Sprintf("cmd %s: %v", cmd.cmd.Name, err))
				} else {
					flaggy.DefaultParser.ShowHelpWithMessage(fmt.Sprintf("cmd %s: %v", cmd.cmd.Name, err))
				}
			}
			return

		}
	}
}

func newGetCmd() *Cmd {
	build := ""
	since := ""
	showChanges := false
	rawJson := false

	get := flaggy.NewSubcommand("get")
	get.Description = "Get Build Metadata"

	get.String(&build, "", "build", "[optional] Build to fetch (defaults to latest)")
	get.Bool(&showChanges, "", "changes", "[optional] show changes")
	get.String(&since, "", "since", "[optional] Fetch all builds between the latest and this one")
	get.Bool(&rawJson, "", "json", "[optional] dump the raw json metadata")

	handler := func(cmd *Cmd) error {
		if paperProjectVersion == "" {
			versions, err := papertool.GetVersions(serverURL, paperProject)
			if err != nil {
				return err
			}
			if len(versions.Versions) == 0 {
				return fmt.Errorf("no versions")
			}
			paperProjectVersion = versions.Versions[len(versions.Versions)-1]
		}

		builds, err := papertool.GetBuilds(serverURL, paperProject, paperProjectVersion)
		if err != nil {
			return err
		}

		if rawJson {
			os.Stdout.Write(builds.Raw())
			return nil
		}

		if len(builds.Builds) == 0 {
			return fmt.Errorf("no builds")
		}

		currentBuildIndex := builds.FindBuildIndex(build)
		if currentBuildIndex < 0 {
			return fmt.Errorf("-build: build '%s' not found", build)
		}

		finalBuildIndex := builds.FindBuildIndex(since)
		if finalBuildIndex < 0 {
			return fmt.Errorf("-since: build '%s' not found", build)
		}

		first := true
		for {
			if !first {
				fmt.Printf("----------\n")
			}

			currentBuild := builds.Builds[currentBuildIndex]

			fmt.Printf("Build    %s\n", papertool.String(currentBuild.Build))
			fmt.Printf("Time     %s\n", papertool.String(currentBuild.Time))
			fmt.Printf("Channel  %s\n", papertool.String(currentBuild.Channel))

			if currentBuild.Artifact != nil && currentBuild.Artifact.Application != nil {
				fmt.Printf("Artifact %s sha256 %s\n", papertool.String(currentBuild.Artifact.Application.Name), papertool.String(currentBuild.Artifact.Application.Sha256))
			}

			if showChanges {
				for _, change := range currentBuild.Changes {
					fmt.Printf("Change %s\n", papertool.String(change.Commit))
					comment := cleanComment(papertool.String(change.Message))
					os.Stdout.WriteString(comment)
				}
			}

			currentBuildIndex--
			if currentBuildIndex < 0 {
				break
			}

			if currentBuildIndex < finalBuildIndex {
				break
			}

			first = false
		}

		if rawJson {
			fmt.Printf("]\n")
		}

		return nil
	}

	return &Cmd{cmd: get, handler: handler}
}

func cleanComment(comment string) string {
	comment = strings.TrimRight(comment, "\n")
	comment = strings.ReplaceAll(comment, "\n", "\n\t")

	return "\t" + comment + "\n"
}

func newDownloadCmd() *Cmd {
	build := ""
	dstdir := ""
	replace := false

	get := flaggy.NewSubcommand("download")
	get.Description = "download build artifact"

	get.String(&build, "", "build", "[optional] Build to fetch (defaults to latest)")
	get.String(&dstdir, "", "dstdir", "[optional] Destination directory to download artifact(s) into")
	get.Bool(&replace, "", "replace", "[optional] replace artifacts if they already exist")

	handler := func(cmd *Cmd) error {
		if paperProjectVersion == "" {
			versions, err := papertool.GetVersions(serverURL, paperProject)
			if err != nil {
				return err
			}
			if len(versions.Versions) == 0 {
				return fmt.Errorf("no versions")
			}
			paperProjectVersion = versions.Versions[len(versions.Versions)-1]
		}

		builds, err := papertool.GetBuilds(serverURL, paperProject, paperProjectVersion)
		if err != nil {
			return err
		}

		if len(builds.Builds) == 0 {
			return fmt.Errorf("no builds")
		}

		buildIndex := builds.FindBuildIndex(build)
		if buildIndex < 0 {
			return fmt.Errorf("-build: build '%s' not found", build)
		}

		if dstdir == "" {
			dstdir = "."
		}

		st, err := os.Stat(dstdir)
		if os.Stat(dstdir); err != nil {
			return fmt.Errorf("%s: %v", dstdir, err)
		}
		if !st.IsDir() {
			return fmt.Errorf("%s: is not a directory", dstdir)
		}

		b := builds.Builds[buildIndex]

		err = papertool.Download(serverURL, paperProject, paperProjectVersion, build, b.Artifact, dstdir, replace, quiet)
		if err != nil {
			return err
		}

		return nil
	}

	return &Cmd{cmd: get, handler: handler}
}
