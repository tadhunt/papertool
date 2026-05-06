package papertool

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

/*
 * PROJECT is one of "paper", "velocity", or "waterfall".
 *
 * As of 2026, papermc.io migrated from the v2 API on api.papermc.io to the
 * v3 "Fill" API on fill.papermc.io. v2 is no longer being updated for new
 * release lines (e.g. velocity 3.5.0-SNAPSHOT only shows up on v3). This
 * package now talks to v3 and translates the responses into the legacy
 * shape so existing callers keep working.
 *
 *
 * v3 endpoints:
 *
 *   GET https://fill.papermc.io/v3/projects/${PROJECT}
 *   {
 *     "project":  { "id": "velocity", "name": "Velocity" },
 *     "versions": { "3.0.0": ["3.5.0-SNAPSHOT", "3.4.0", ...], ... }
 *   }
 *
 *   GET https://fill.papermc.io/v3/projects/${PROJECT}/versions/${VERSION}/builds
 *   [
 *     {
 *       "id": 594,
 *       "time": "2026-05-01T18:17:18.376Z",
 *       "channel": "STABLE",
 *       "commits": [{ "sha": "...", "time": "...", "message": "..." }, ...],
 *       "downloads": {
 *         "server:default": {
 *           "name": "velocity-3.5.0-SNAPSHOT-594.jar",
 *           "checksums": { "sha256": "..." },
 *           "size": 18881365,
 *           "url":  "https://fill-data.papermc.io/v1/objects/.../velocity-3.5.0-SNAPSHOT-594.jar"
 *         }
 *       }
 *     },
 *     ...
 *   ]
 *
 *   GET https://fill.papermc.io/v3/projects/${PROJECT}/versions/${VERSION}/builds/${BUILD}
 *   (single build object, same shape as one element of the array above)
 *
 *
 * Legacy ordering note: the v2 versions and builds responses were
 * oldest-first. v3 returns versions per group newest-first within each
 * group, and builds newest-first. We reverse on translation so callers
 * that pick `Versions[len-1]` / `Builds[len-1]` for "latest" still work.
 */

const (
	Project_Paper     = "paper"
	Project_Velocity  = "velocity"
	Project_Waterfall = "waterfall"
)

// --- Public types (kept stable across the v2 → v3 transition). ---

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
	// URL is the direct download URL provided by the v3 API. Set by the
	// translation layer; empty for callers that constructed an Artifact
	// some other way.
	URL *string `json:"url,omitempty"`
}

type MetadataSyntaxError struct {
	Raw    string
	msg    string
	Offset int64
}

func (e *MetadataSyntaxError) Error() string {
	return e.msg
}

// --- v3 raw response shapes (private). ---

type v3Project struct {
	Project struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
	Versions map[string][]string `json:"versions"`
}

type v3Build struct {
	ID        json.Number             `json:"id"`
	Time      string                  `json:"time"`
	Channel   string                  `json:"channel"`
	Commits   []v3Commit              `json:"commits"`
	Downloads map[string]v3Download   `json:"downloads"`
}

type v3Commit struct {
	Sha     string `json:"sha"`
	Time    string `json:"time"`
	Message string `json:"message"`
}

type v3Download struct {
	Name      string `json:"name"`
	Checksums struct {
		Sha256 string `json:"sha256"`
	} `json:"checksums"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

// --- Public fetchers (now hitting v3). ---

func GetVersions(src *url.URL, project string) (*Versions, error) {
	u := fmt.Sprintf("%s/v3/projects/%s", src.String(), project)

	v3 := &v3Project{}
	raw, err := fetch(u, v3)
	if err != nil {
		return nil, err
	}

	pid := v3.Project.ID
	pname := v3.Project.Name
	versions := &Versions{
		ProjectID:   &pid,
		ProjectName: &pname,
		raw:         raw,
	}

	groups := make([]string, 0, len(v3.Versions))
	for g := range v3.Versions {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return semverLess(groups[i], groups[j]) })
	versions.VersionGroups = groups

	for _, g := range groups {
		gv := v3.Versions[g]
		// v3 lists versions newest-first within each group; reverse so the
		// flattened list is oldest-first (newest at len-1), matching v2.
		for i := len(gv) - 1; i >= 0; i-- {
			versions.Versions = append(versions.Versions, gv[i])
		}
	}

	return versions, nil
}

func GetBuilds(src *url.URL, project string, version string) (*Builds, error) {
	u := fmt.Sprintf("%s/v3/projects/%s/versions/%s/builds", src.String(), project, version)

	var v3Builds []v3Build
	raw, err := fetch(u, &v3Builds)
	if err != nil {
		return nil, err
	}

	pid := project
	ver := version
	builds := &Builds{
		ProjectID: &pid,
		Version:   &ver,
		raw:       raw,
	}

	// v3 returns newest-first; reverse for oldest-first / newest-last.
	for i := len(v3Builds) - 1; i >= 0; i-- {
		b, err := v3BuildToLegacy(&v3Builds[i])
		if err != nil {
			return nil, err
		}
		b.ProjectID = builds.ProjectID
		builds.Builds = append(builds.Builds, b)
	}

	return builds, nil
}

func GetBuild(src *url.URL, project string, version string, build string) (*Build, error) {
	u := fmt.Sprintf("%s/v3/projects/%s/versions/%s/builds/%s", src.String(), project, version, build)

	v3 := &v3Build{}
	raw, err := fetch(u, v3)
	if err != nil {
		return nil, err
	}

	b, err := v3BuildToLegacy(v3)
	if err != nil {
		return nil, err
	}
	pid := project
	b.ProjectID = &pid
	b.raw = raw
	return b, nil
}

// v3BuildToLegacy translates a v3 build record into the legacy v2-shaped
// Build. The "server:default" download is preferred; if absent, any other
// download key starting with "server:" is used.
func v3BuildToLegacy(v3 *v3Build) (*Build, error) {
	id, err := v3.ID.Float64()
	if err != nil {
		return nil, fmt.Errorf("parse build id %q: %w", v3.ID.String(), err)
	}
	t := v3.Time
	ch := v3.Channel
	b := &Build{
		Build:   &id,
		Time:    &t,
		Channel: &ch,
	}
	for _, c := range v3.Commits {
		sha := c.Sha
		// v3 doesn't separate summary from message; populate both with
		// message so callers checking either field still see content.
		msg := c.Message
		b.Changes = append(b.Changes, &Change{
			Commit:  &sha,
			Summary: &msg,
			Message: &msg,
		})
	}

	var dl *v3Download
	if d, ok := v3.Downloads["server:default"]; ok {
		dl = &d
	} else {
		// Stable fallback: pick whichever "server:*" key sorts first so
		// the result is deterministic across runs.
		keys := make([]string, 0, len(v3.Downloads))
		for k := range v3.Downloads {
			if strings.HasPrefix(k, "server:") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			d := v3.Downloads[keys[0]]
			dl = &d
		}
	}

	if dl != nil {
		name := dl.Name
		sha := dl.Checksums.Sha256
		dlurl := dl.URL
		b.Artifact = &Artifact{
			Application: &Application{
				Name:   &name,
				Sha256: &sha,
				URL:    &dlurl,
			},
		}
	}

	return b, nil
}

// semverLess compares dotted-numeric version-group keys (e.g. "1.0.0" vs
// "3.0.0"). Non-numeric segments compare lexicographically as a fallback.
func semverLess(a, b string) bool {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	n := len(aParts)
	if len(bParts) < n {
		n = len(bParts)
	}
	for i := 0; i < n; i++ {
		ai, _ := strconv.Atoi(aParts[i])
		bi, _ := strconv.Atoi(bParts[i])
		if ai != bi {
			return ai < bi
		}
	}
	return len(aParts) < len(bParts)
}

func fetch(src string, result any) ([]byte, error) {
	response, err := http.Get(src)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: status %d: %s", src, response.StatusCode, strings.TrimSpace(string(body)))
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
