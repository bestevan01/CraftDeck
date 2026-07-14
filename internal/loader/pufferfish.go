package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// pufferfishJenkinsBase is Pufferfish's Jenkins CI server -- Pufferfish (a
// high-performance Paper fork) has no official REST distribution API like
// Paper/Purpur, only Jenkins build artifacts (verified against
// ci.pufferfish.host/api/json). There's one job per Minecraft *minor* line
// (e.g. "Pufferfish-1.21" covers every 1.21.x patch as that line's builds
// roll forward over time), unlike Paper's fill API which keeps every exact
// patch version's build addressable forever.
const pufferfishJenkinsBase = "https://ci.pufferfish.host"

// pufferfishJobNameRE matches only the plain per-line jobs ("Pufferfish-1.21"),
// excluding variants like "Pufferfish-Purpur-1.18" that show up in the same
// job list.
var pufferfishJobNameRE = regexp.MustCompile(`^Pufferfish-[0-9]+(\.[0-9]+)*$`)

// pufferfishVersionInFilenameRE pulls the Minecraft version back out of an
// artifact filename -- naming isn't consistent across jobs (observed
// "Pufferfish-1.17.1-R0.1-SNAPSHOT.jar", "pufferfish-paperclip-1.18.2-R0.1-
// SNAPSHOT-reobf.jar", "pufferfish-paperclip-1.21.10-R0.1-SNAPSHOT-mojmap.jar"),
// but the version number itself is always the first dotted number group.
var pufferfishVersionInFilenameRE = regexp.MustCompile(`[0-9]+\.[0-9]+(?:\.[0-9]+)?`)

type PufferfishAdapter struct{}

type pufferfishJenkinsJobList struct {
	Jobs []struct {
		Name string `json:"name"`
	} `json:"jobs"`
}

type pufferfishJenkinsBuild struct {
	URL       string `json:"url"`
	Artifacts []struct {
		FileName     string `json:"fileName"`
		RelativePath string `json:"relativePath"`
	} `json:"artifacts"`
}

// pufferfishJobName derives the Jenkins job a Minecraft version would be
// built under from its major.minor prefix (e.g. "1.21.10" -> "Pufferfish-1.21").
func pufferfishJobName(mcVersion string) string {
	parts := strings.SplitN(mcVersion, ".", 3)
	if len(parts) < 2 {
		return "Pufferfish-" + mcVersion
	}
	return "Pufferfish-" + parts[0] + "." + parts[1]
}

// pufferfishLatestBuild fetches jobName's most recent successful build and
// the exact Minecraft version its (single) artifact was actually built for
// -- the job's own name only pins down the minor line, not the patch.
func pufferfishLatestBuild(ctx context.Context, jobName string) (*pufferfishJenkinsBuild, string, error) {
	build, err := getJSON[pufferfishJenkinsBuild](ctx, fmt.Sprintf("%s/job/%s/lastSuccessfulBuild/api/json", pufferfishJenkinsBase, jobName))
	if err != nil {
		return nil, "", err
	}
	if len(build.Artifacts) == 0 {
		return nil, "", fmt.Errorf("%s's latest build has no artifacts", jobName)
	}
	version := pufferfishVersionInFilenameRE.FindString(build.Artifacts[0].FileName)
	if version == "" {
		return nil, "", fmt.Errorf("couldn't find a version number in %q", build.Artifacts[0].FileName)
	}
	return build, version, nil
}

// compareVersions orders dotted numeric version strings (e.g. "1.9" <
// "1.21"), unlike a plain string comparison which would get that backwards.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var an, bn int
		if i < len(as) {
			an, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bn, _ = strconv.Atoi(bs[i])
		}
		if an != bn {
			return an - bn
		}
	}
	return 0
}

// FetchPufferfishVersions lists the exact Minecraft version each of
// Pufferfish's active Jenkins jobs currently builds, newest first. Unlike
// Paper/Purpur/Folia, this is necessarily just one version per minor line
// (whatever that line's latest build happens to target right now), not
// every patch version ever released -- see Download's staleness check for
// why that distinction matters.
func FetchPufferfishVersions(ctx context.Context) ([]string, error) {
	list, err := getJSON[pufferfishJenkinsJobList](ctx, pufferfishJenkinsBase+"/api/json")
	if err != nil {
		return nil, fmt.Errorf("list pufferfish jenkins jobs: %w", err)
	}

	var versions []string
	for _, job := range list.Jobs {
		if !pufferfishJobNameRE.MatchString(job.Name) {
			continue // e.g. "Pufferfish-Purpur-1.18", a different variant
		}
		_, version, err := pufferfishLatestBuild(ctx, job.Name)
		if err != nil {
			continue // one job's CI hiccup shouldn't blank the whole list
		}
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return compareVersions(versions[i], versions[j]) > 0 })
	return versions, nil
}

func (PufferfishAdapter) Download(ctx context.Context, mcVersion string, destDir string) (string, error) {
	jobName := pufferfishJobName(mcVersion)
	build, actualVersion, err := pufferfishLatestBuild(ctx, jobName)
	if err != nil {
		return "", fmt.Errorf("fetch latest pufferfish build for %q: %w", mcVersion, err)
	}
	// The job only ever exposes its minor line's single latest build, so by
	// the time this runs (possibly long after the version list was loaded)
	// it may have already moved on to a newer patch than what was selected.
	if actualVersion != mcVersion {
		return "", fmt.Errorf(
			"pufferfish의 %s 빌드가 그 사이 %s로 갱신되어 더 이상 받을 수 없습니다 -- 버전 목록을 새로고침 후 %s를 선택하세요",
			mcVersion, actualVersion, actualVersion,
		)
	}

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return "", fmt.Errorf("create work dir: %w", err)
	}
	jarPath := filepath.Join(destDir, "server.jar")
	downloadURL := build.URL + "artifact/" + build.Artifacts[0].RelativePath
	// Jenkins doesn't publish a checksum alongside the artifact the way
	// Paper/Purpur/Vanilla's APIs do, so there's nothing to verify against.
	if err := downloadAndVerify(ctx, downloadURL, nil, "", jarPath); err != nil {
		return "", err
	}
	return jarPath, nil
}
