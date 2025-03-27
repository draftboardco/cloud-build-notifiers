package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"text/template"

	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
)

const blockKitTemplate = `[
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "Cloud Build {{.Build.ProjectId}} {{.Build.Id}} {{.Build.Status}}"
      }
    },
    {{if .Build.Source}}
    {{/* Git Source */}}
    {{with .Build.Source.Source}}
    {{if (typeOf .) | eq "*cloudbuildpb.Source_GitSource"}}
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*GitHub Repository:*\n• Repository: {{$.Build.Substitutions.REPO_FULL_NAME}}\n• Commit: {{.GitSource.Revision}}\n• Branch: {{$.Build.Substitutions.BRANCH_NAME}} ({{$.Build.Substitutions.SHORT_SHA}})"
      }
    },
    {{/* Storage Source */}}
    {{else if (typeOf .) | eq "*cloudbuildpb.Source_StorageSource"}}
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Cloud Storage Source:*\n• Bucket: gs://{{.StorageSource.Bucket}}\n• Object: {{.StorageSource.Object}}\n• Generation: {{.StorageSource.Generation}}"
      }
    },
    {{end}}
    {{end}}
    {{end}}
    {{/* Deployment Information */}}
    {{if .Build.Substitutions}}
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "*Deployment Information:*{{if or (contains .Build.Substitutions._CLUSTER "prod") (contains .Build.Substitutions._NAMESPACE "prod")}} ⚠️ *PRODUCTION*{{end}}\n• Project: {{if .Build.Substitutions._CLUSTER_PROJECT}}{{.Build.Substitutions._CLUSTER_PROJECT}}{{else}}{{.Build.ProjectId}}{{end}}\n• Cluster: {{.Build.Substitutions._CLUSTER}}{{if .Build.Substitutions._LOCATION}}\n• Location: {{.Build.Substitutions._LOCATION}}{{end}}\n• Namespace: {{.Build.Substitutions._NAMESPACE}}"
      }
    },
    {{end}}
    {
      "type": "divider"
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": "View Build Logs"
      },
      "accessory": {
        "type": "button",
        "text": {
          "type": "plain_text",
          "text": "Logs"
        },
        "value": "click_me_123",
        "url": "{{.Build.LogUrl}}",
        "action_id": "button-action"
      }
    }
  ]`

func TestWriteMessage(t *testing.T) {
	tests := []struct {
		name     string
		build    *cbpb.Build
		wantText []string
	}{
		{
			name: "build_with_git_source",
			build: &cbpb.Build{
				ProjectId: "draftboard-368620",
				Id:        "177e4613-0964-426d-9d6f-bbe677a7d6da",
				Status:    cbpb.Build_SUCCESS,
				Source: &cbpb.Source{
					Source: &cbpb.Source_GitSource{
						GitSource: &cbpb.GitSource{
							Url:      "https://github.com/draftboardco/sales-intro-svc.git",
							Revision: "b8decbd7e1df6dbb2e075027d502088105eec68d",
						},
					},
				},
				Substitutions: map[string]string{
					"REPO_FULL_NAME":   "draftboardco/sales-intro-svc",
					"BRANCH_NAME":      "develop",
					"SHORT_SHA":        "b8decbd",
					"_CLUSTER":         "sales-dev-1",
					"_CLUSTER_PROJECT": "clear-rock-393213",
					"_NAMESPACE":       "dev",
				},
			},
			wantText: []string{
				"*GitHub Repository:*",
				"Repository: draftboardco/sales-intro-svc",
				"Branch: develop (b8decbd)",
				"Project: clear-rock-393213",
				"Cluster: sales-dev-1",
				"Namespace: dev",
			},
		},
		{
			name: "build_with_storage_source",
			build: &cbpb.Build{
				ProjectId: "draftboard-368620",
				Id:        "7134be45-389a-4eae-86bd-c049eaa310a9",
				Status:    cbpb.Build_WORKING,
				Source: &cbpb.Source{
					Source: &cbpb.Source_StorageSource{
						StorageSource: &cbpb.StorageSource{
							Bucket:     "draftboard-368620_cloudbuild",
							Object:     "source/1742557736.879493-8dc3d43c28ef4a3b8d9d81bd9264fae8.tgz",
							Generation: 1742557737797427,
						},
					},
				},
				Substitutions: map[string]string{
					"_CLUSTER":         "sales-prod-1",
					"_CLUSTER_PROJECT": "draftboard-368620",
					"_LOCATION":        "us-central1-c",
					"_NAMESPACE":       "s-prod",
					"_TAG":             "v0.15.0",
				},
			},
			wantText: []string{
				"*Cloud Storage Source:*",
				"Bucket: gs://draftboard-368620_cloudbuild",
				"Object: source/1742557736.879493-8dc3d43c28ef4a3b8d9d81bd9264fae8.tgz",
				"Generation: 1742557737797427",
				"⚠️ *PRODUCTION*",
				"Project: draftboard-368620",
				"Cluster: sales-prod-1",
				"Location: us-central1-c",
				"Namespace: s-prod",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sn := &slackNotifier{
				tmpl: template.Must(template.New("test").Funcs(template.FuncMap{
					"contains": strings.Contains,
					"typeOf": func(v interface{}) string {
						return fmt.Sprintf("%T", v)
					},
					"eq": func(a, b interface{}) bool {
						return a == b
					},
				}).Parse(blockKitTemplate)),
			}

			sn.tmplView = &notifiers.TemplateView{
				Build: &notifiers.BuildView{Build: tc.build},
			}

			msg, err := sn.writeMessage()
			if err != nil {
				t.Fatalf("writeMessage got error: %v", err)
			}

			msgJSON, err := json.Marshal(msg)
			if err != nil {
				t.Fatalf("Failed to marshal message: %v", err)
			}
			msgStr := string(msgJSON)

			for _, want := range tc.wantText {
				if !strings.Contains(msgStr, want) {
					t.Errorf("Message does not contain %q\nGot message: %s", want, msgStr)
				}
			}
		})
	}
}
