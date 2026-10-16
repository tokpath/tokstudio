package modelruntime

import (
	"net/url"
	"testing"
)

func TestTaskEndpointIsOneEscapedPathSegment(t *testing.T) {
	const template = "https://provider.example.test/v1/videos/{task_id}/content"
	for _, task := range []string{"video_123", "folder/id", "x?target=other#fragment", "https://other.example.test/", "a%b"} {
		resolved, err := ResolveTaskEndpoint(template, task)
		if err != nil {
			t.Fatal(task, err)
		}
		u, err := url.Parse(resolved)
		if err != nil || u.Host != "provider.example.test" || u.RawQuery != "" || u.Fragment != "" || u.EscapedPath() != "/v1/videos/"+url.PathEscape(task)+"/content" {
			t.Fatal("task escaped its configured path", resolved, err)
		}
	}
	for _, bad := range []string{"https://{task_id}/videos", "https://provider.example.test/v1/videos?task={task_id}", "https://provider.example.test/v1/{task_id}-suffix", "https://provider.example.test/v1/{task_id}/{task_id}", "https://user@provider.example.test/v1/{task_id}", "https://provider.example.test/v1/../{task_id}", "https://provider.example.test/v1/%7Btask_id%7D", "https://provider.example.test/v1/{task_id}#x"} {
		if _, err := ResolveTaskEndpoint(bad, "video_1"); err == nil {
			t.Fatal("unsafe template accepted", bad)
		}
	}
	for _, bad := range []string{"", ".", "..", "x\ny"} {
		if _, err := ResolveTaskEndpoint(template, bad); err == nil {
			t.Fatal("unsafe task accepted")
		}
	}
}
