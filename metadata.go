package papertool

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

/*
 * PROJECT is one of "paper", "velocity", or "waterfall"
 */

/*
 * Fetch project release channels
 *	GET https://api.papermc.io/v2/projects/${PROJECT}
 *
 * Response:
 * {
 *     "project_id": "velocity",
 *     "project_name": "Velocity",
 *     "version_groups": [
 *         "1.0.0",
 *         "1.1.0",
 *         "3.0.0"
 *     ],
 *     "versions": [											<-- These are release channels
 *         "1.0.10",
 *         "1.1.9",
 *         "3.1.0",
 *         "3.1.1",
 *         "3.1.1-SNAPSHOT",
 *         "3.1.2-SNAPSHOT",
 *         "3.2.0-SNAPSHOT"
 *     ]
 * }
 */

/*
 * Fetch build info for the given ${PROJECT} and ${CHANNEL}
 *	GET https://api.papermc.io/v2/projects/${PROJECT}/versions/${CHANNEL}/builds
 *
 * For example:
 *	GET https://api.papermc.io/v2/projects/velocity/versions/3.2.0-SNAPSHOT/builds
 *
 * Response:
 * {
 *     "project_id": "velocity",
 *     "project_name": "Velocity",
 *     "version": "3.2.0-SNAPSHOT",
 *     "builds": [
 *         {
 *             "build": 214,
 *             "time": "2023-01-02T02:52:34.092Z",
 *             "channel": "default",
 *             "promoted": false,
 *             "changes": [
 *                 {
 *                     "commit": "1bfeac58b6069a061326d3ced1940e3ccf5feb18",
 *                     "summary": "all, not just sub",
 *                     "message": "all, not just sub\n"
 *                 }
 *             ],
 *             "downloads": {
 *                 "application": {
 *                     "name": "velocity-3.2.0-SNAPSHOT-214.jar",					<-- ARTIFACT
 *                     "sha256": "1bf681f954bc4d68a3b395c1eb360a933500f8fd960679bf40e8d05af16e8483"
 *                 }
 *             }
 *         },
 * 	[...]
 */

/*
 * Download a build artifact
 *	GET https://api.papermc.io/v2/projects/${PROJECT}/versions/${CHANNEL}/builds/${BUILD}/downloads/${ARTIFACT}
 *
 * For example:
 *	GET https://api.papermc.io/v2/projects/velocity/versions/3.2.0-SNAPSHOT/builds/261/downloads/velocity-3.2.0-SNAPSHOT-261.jar
 *
 * Response:
 *	The requested file
 */

const (
	Project_Paper     = "paper"
	Project_Velocity  = "velocity"
	Project_Waterfall = "waterfall"
)

type Channels struct {
	ID            *string  `json:"project_id"`
	Name          *string  `json:"project_name"`
	VersionGroups []string `json:"version_groups"`
	Versions      []string `json:"versions"`
}

type Builds struct {
	ID      *string  `json:"project_id"`
	Name    *string  `json:"project_name"`
	Version *string  `json:"version"`
	Builds  []*Build `json:"builds"`
}

type Build struct {
	Build     *string     `json:"build"`
	Time      *string     `json:"time"`
	Channel   *string     `json:"channel"`
	Promoted  *bool       `json:"promoted"`
	Changes   []*Change   `json:"changes"`
	Downloads []*Artifact `json:"downloads"`
}

type Change struct {
	Commit  *string `json:"commit"`
	Summary *string `json:"summary"`
	Message *string `json:"message"`
}

type Artifact struct {
	Application *Application `json:"application"`
}

type Application struct {
	Name   *string `json:"name"`
	Sha256 *string `json:"sha256"`
}

type MetadataSyntaxError struct {
	Raw    string
	msg    string
	Offset int64
}

func (e *MetadataSyntaxError) Error() string {
	return e.msg
}

func GetChannels(src *url.URL, project string) (channels *Channels, err error) {
	u := fmt.Sprintf("%s/api/v2/projects/%s", src.String(), project)

	err = fetch(u, channels)
	return
}
	
func GetBuilds(src *url.URL, project string, channel string) (builds *Builds, err error) {
	u := fmt.Sprintf("%s/api/v2/projects/%s/versions/%s/builds", src.String(), project, channel)
	err = fetch(u, builds)
	return
}

func GetBuild(src *url.URL, project string, channel string, build string) (b *Build, err error) {
	u := fmt.Sprintf("%s/api/v2/projects/%s/versions/%s/builds/%s", src.String(), project, channel, build)

	err = fetch(u, b)
	return
}

func GetRawBuildMetadata(src *url.URL, build string) ([]byte, error) {
	build = parseBuild(build)

	u := fmt.Sprintf("%s/%s/api/json", src.String(), build)

	response, err := http.Get(u)
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func fetch(src string, result any) error {
	response, err := http.Get(src)
	if err != nil {
		return err
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	err = unmarshal(body, result)
	if err != nil {
		return err
	}

	return nil
}

func unmarshal(raw []byte, dst any) error {
	err := json.Unmarshal(raw, dst)
	if err != nil {
		serr, isSyntaxError := err.(*json.SyntaxError)
		if isSyntaxError {
			return &MetadataSyntaxError{
				Raw:    string(raw),
				msg:    fmt.Sprintf("%v (offset %d)", serr, serr.Offset),
				Offset: serr.Offset,
			}
		}
		return err
	}

	return nil
}
