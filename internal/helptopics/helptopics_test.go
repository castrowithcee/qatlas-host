package helptopics

import (
	"regexp"
	"strings"
	"testing"
)

func TestTopicsAreValid(t *testing.T) {
	name := regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	seen := map[string]bool{}
	for _, topic := range All() {
		if !name.MatchString(topic.Name) || seen[topic.Name] {
			t.Errorf("invalid or repeated topic name %q", topic.Name)
		}
		seen[topic.Name] = true
		if topic.Short == "" || topic.Text == "" {
			t.Errorf("topic %q needs a short help and a text", topic.Name)
		}
	}
}

func TestSourcesTopicNamesTheGitCheckAndNoLongerAdvisesGroupAccess(t *testing.T) {
	for _, want := range []string{"git-unsafe", "source secure", "only for root"} {
		if !strings.Contains(sourcesTopic, want) {
			t.Errorf("the sources topic lacks %q", want)
		}
	}
	for _, gone := range []string{"safe.directory", "umask 002", "usermod", "sudo -u qhost-deploy"} {
		if strings.Contains(sourcesTopic, gone) {
			t.Errorf("the sources topic still names %q", gone)
		}
	}
}
