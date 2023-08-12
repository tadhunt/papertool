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
 * Fetch builds for the given ${PROJECT} and ${VERSION}
 *	GET https://api.papermc.io/v2/projects/${PROJECT}/versions/${VERSION}/builds
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
 * Fetch a specific build for the given ${PROJECT} and ${VERSION}
 *	GET https://api.papermc.io/v2/projects/${PROJECT}/versions/${VERSION}/builds/${BUILD}
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
 *	GET https://api.papermc.io/v2/projects/${PROJECT}/versions/${VERSION}/builds/${BUILD}/downloads/${ARTIFACT}
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

type Versions struct {
	ProjectID     *string  `json:"project_id"`
	ProjectName   *string  `json:"project_name"`
	VersionGroups []string `json:"version_groups"`
	Versions      []string `json:"versions"`
	raw           []byte
}

type Builds struct {
	ProjectID   *string  `json:"project_id"`
	ProjectName *string  `json:"project_name"`
	Version     *string  `json:"version"`
	Builds      []*Build `json:"builds"`
	raw         []byte
}

type Build struct {
	ProjectID   *string   `json:"project_id"`
	ProjectName *string   `json:"project_name"`
	Build       *float64  `json:"build"`
	Time        *string   `json:"time"`
	Channel     *string   `json:"channel"`
	Promoted    *bool     `json:"promoted"`
	Changes     []*Change `json:"changes"`
	Artifact    *Artifact `json:"downloads"`
	raw         []byte
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

func GetVersions(src *url.URL, project string) (versions *Versions, err error) {
	u := fmt.Sprintf("%s/v2/projects/%s", src.String(), project)

	versions = &Versions{}
	versions.raw, err = fetch(u, versions)
	return
}

func GetBuilds(src *url.URL, project string, version string) (builds *Builds, err error) {
	u := fmt.Sprintf("%s/v2/projects/%s/versions/%s/builds", src.String(), project, version)

	builds = &Builds{}
	builds.raw, err = fetch(u, builds)
	if err != nil {
		return
	}

	//
	// these fields are populated when fetching a specific build, but not when fetching all builds
	//
	for _, build := range builds.Builds {
		build.ProjectID = builds.ProjectID
		build.ProjectName = builds.ProjectName
	}

	return
}

func GetBuild(src *url.URL, project string, version string, build string) (b *Build, err error) {
	u := fmt.Sprintf("%s/v2/projects/%s/versions/%s/builds/%s", src.String(), project, version, build)

	b = &Build{}
	b.raw, err = fetch(u, b)
	return
}

func fetch(src string, result any) ([]byte, error) {
	response, err := http.Get(src)
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	err = unmarshal(body, result)
	if err != nil {
		return nil, err
	}

	return body, nil
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

func (builds *Builds) FindBuildIndex(build string) int {
	if len(builds.Builds) == 0 {
		return -1
	}

	if build == "" || build == "latest" {
		return len(builds.Builds) - 1
	}

	if build == "first" {
		return 0
	}

	for i, b := range builds.Builds {
		if String(b.Build) == build {
			return i
		}
	}

	return -1
}

func (builds *Builds) FindBuild(build string) *Build {
	i := builds.FindBuildIndex(build)
	if i < 0 {
		return nil
	}

	return builds.Builds[i]
}

func (versions *Versions) Raw() []byte {
	return versions.raw
}

func (builds *Builds) Raw() []byte {
	return builds.raw
}

func (build *Build) Raw() []byte {
	return build.raw
}
