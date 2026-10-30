package promotion

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"

	kargoapi "github.com/akuity/kargo/api/v1alpha1"
)

// isRollback returns true if the Promotion is explicitly marked as a rollback,
// which is the only way to promote Freight that would downgrade a Stage.
func isRollback(promo *kargoapi.Promotion) bool {
	return promo.Annotations[kargoapi.AnnotationKeyRollback] ==
		kargoapi.AnnotationValueTrue
}

// findDowngrades compares the target Freight with the Freight from the same
// origin that is currently in the Stage, and returns a human-readable entry for
// every artifact whose version would go down.
//
// Artifacts are matched by identity within the origin: images by repository
// URL, charts by repository URL and name, commits by repository URL, generic
// artifacts by subscription name. Images, charts and commits must also come
// from the same subscription, since a Warehouse may subscribe to one
// repository more than once (e.g. two tag lines); Freight that predates
// subscription names has none on either side and still matches.
//
// Only semantic versions are compared -- an image tag, a chart version, a
// commit's tag, an artifact's version. A pair
// that is not comparable (no counterpart in the current Freight, or either side
// not a semantic version) is never reported, so the check cannot block a
// promotion it cannot judge.
func findDowngrades(
	current *kargoapi.FreightReference,
	target *kargoapi.Freight,
) []string {
	if current == nil || target == nil {
		return nil
	}
	var downgrades []string
	check := func(what, from, to string) {
		if isDowngrade(from, to) {
			downgrades = append(
				downgrades,
				fmt.Sprintf("%s %s -> %s", what, from, to),
			)
		}
	}

	for _, img := range target.Images {
		for _, cur := range current.Images {
			if cur.RepoURL == img.RepoURL &&
				cur.SubscriptionName == img.SubscriptionName {
				check("image "+img.RepoURL, cur.Tag, img.Tag)
			}
		}
	}
	for _, chart := range target.Charts {
		for _, cur := range current.Charts {
			if cur.RepoURL == chart.RepoURL && cur.Name == chart.Name &&
				cur.SubscriptionName == chart.SubscriptionName {
				check("chart "+chartID(chart), cur.Version, chart.Version)
			}
		}
	}
	for _, commit := range target.Commits {
		for _, cur := range current.Commits {
			if cur.RepoURL == commit.RepoURL &&
				cur.SubscriptionName == commit.SubscriptionName {
				check("git "+commit.RepoURL, cur.Tag, commit.Tag)
			}
		}
	}
	for _, artifact := range target.Artifacts {
		for _, cur := range current.Artifacts {
			if cur.SubscriptionName == artifact.SubscriptionName {
				check(
					"artifact "+artifact.SubscriptionName,
					cur.Version,
					artifact.Version,
				)
			}
		}
	}
	return downgrades
}

// isDowngrade returns true if both versions are semantic versions and to is
// lower than from.
func isDowngrade(from, to string) bool {
	if from == "" || to == "" || from == to {
		return false
	}
	fromVer, err := semver.NewVersion(from)
	if err != nil {
		return false
	}
	toVer, err := semver.NewVersion(to)
	if err != nil {
		return false
	}
	return toVer.LessThan(fromVer)
}

func chartID(chart kargoapi.Chart) string {
	if chart.Name == "" {
		return chart.RepoURL
	}
	return strings.TrimSuffix(chart.RepoURL, "/") + "/" + chart.Name
}
