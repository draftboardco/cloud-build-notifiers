package main

import (
	"os"
	"strings"
	"testing"
	"text/template"

	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	"github.com/google/go-cmp/cmp"
	"github.com/slack-go/slack"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/protoadapt"
)

func loadBuildData(t *testing.T, filename string) *cbpb.Build {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read build data file %s: %v", filename, err)
	}

	build := new(cbpb.Build)
	// Be as lenient as possible in unmarshalling.
	// `Unmarshal` will fail if we get a payload with a field that is unknown to the current proto version unless `DiscardUnknown` is set.
	uo := protojson.UnmarshalOptions{
		AllowPartial:   true,
		DiscardUnknown: true,
	}
	bv2 := protoadapt.MessageV2Of(build)
	if err := uo.Unmarshal(data, bv2); err != nil {
		t.Fatalf("failed to unmarshal build data: %v", err)
	}
	build = protoadapt.MessageV1Of(bv2).(*cbpb.Build)

	return build
}

func TestWriteMessage_RealData(t *testing.T) {
	tests := []struct {
		name      string
		buildFile string
		want      *slack.WebhookMessage
	}{
		{
			name:      "successful_build",
			buildFile: "test-data/build.2.json",
			want: &slack.WebhookMessage{
				Attachments: []slack.Attachment{{
					Color: "#22bb33",
					Blocks: slack.Blocks{
						BlockSet: []slack.Block{
							&slack.SectionBlock{
								Type: "section",
								Text: &slack.TextBlockObject{
									Type: "mrkdwn",
									Text: "Cloud Build: Status: SUCCESS",
								},
							},
							&slack.SectionBlock{
								Type: "section",
								Text: &slack.TextBlockObject{
									Type: "mrkdwn",
									Text: "Source: Git\nRepository: draftboardco/sales-intro-svc\nBranch: develop\nCommit: `b8decbd7e1df6dbb2e075027d502088105eec68d`",
								},
							},
							&slack.SectionBlock{
								Type: "section",
								Text: &slack.TextBlockObject{
									Type: "mrkdwn",
									Text: "Target cluster: *clear-rock-393213* \nnamespace: *dev*",
								},
							},
							&slack.DividerBlock{
								Type: "divider",
							},
							&slack.SectionBlock{
								Type: "section",
								Text: &slack.TextBlockObject{
									Type: "mrkdwn",
									Text: "View Build Logs",
								},
								Accessory: &slack.Accessory{ButtonElement: &slack.ButtonBlockElement{
									Type:     "button",
									Text:     &slack.TextBlockObject{Type: "plain_text", Text: "Logs"},
									ActionID: "button-action",
									URL:      "https://console.cloud.google.com/cloud-build/builds;region=us-central1/177e4613-0964-426d-9d6f-bbe677a7d6da?project=354610194356",
									Value:    "click_me_123",
								}},
							},
						},
					},
				}},
			},
		},
		{
			name:      "working_build",
			buildFile: "test-data/build.1.json",
			want: &slack.WebhookMessage{
				Attachments: []slack.Attachment{{
					Color: "#f0ad4e",
					Blocks: slack.Blocks{
						BlockSet: []slack.Block{
							&slack.SectionBlock{
								Type: "section",
								Text: &slack.TextBlockObject{
									Type: "mrkdwn",
									Text: "Cloud Build: Status: WORKING",
								},
							},
							&slack.SectionBlock{
								Type: "section",
								Text: &slack.TextBlockObject{
									Type: "mrkdwn",
									Text: "Source: Storage\nBucket: draftboard-368620_cloudbuild\nObject: source/1742557736.879493-8dc3d43c28ef4a3b8d9d81bd9264fae8.tgz",
								},
							},
							&slack.SectionBlock{
								Type: "section",
								Text: &slack.TextBlockObject{
									Type: "mrkdwn",
									Text: "* :warning: Important :warning: * : This is a production build. \nTarget cluster: *draftboard-368620* \nnamespace: *s-prod*",
								},
							},
							&slack.DividerBlock{
								Type: "divider",
							},
							&slack.SectionBlock{
								Type: "section",
								Text: &slack.TextBlockObject{
									Type: "mrkdwn",
									Text: "View Build Logs",
								},
								Accessory: &slack.Accessory{ButtonElement: &slack.ButtonBlockElement{
									Type:     "button",
									Text:     &slack.TextBlockObject{Type: "plain_text", Text: "Logs"},
									ActionID: "button-action",
									URL:      "https://console.cloud.google.com/cloud-build/builds;region=us-central1/7134be45-389a-4eae-86bd-c049eaa310a9?project=354610194356",
									Value:    "click_me_123",
								}},
							},
						},
					},
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := new(slackNotifier)

			// Read the template from slack.json
			blockKitTemplate, err := os.ReadFile("slack.json")
			if err != nil {
				t.Fatalf("failed to read slack.json: %v", err)
			}

			tmpl, err := template.New("blockkit_template").Funcs(template.FuncMap{
				"replace": func(s, old, new string) string {
					return strings.ReplaceAll(s, old, new)
				},
			}).Parse(string(blockKitTemplate))
			if err != nil {
				t.Fatalf("failed to parse template: %v", err)
			}
			n.tmpl = tmpl

			// Load build data from the JSON file
			build := loadBuildData(t, tt.buildFile)
			n.tmplView = &notifiers.TemplateView{Build: &notifiers.BuildView{Build: build}}

			got, err := n.writeMessage()
			if err != nil {
				t.Fatalf("writeMessage failed: %v", err)
			}

			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("writeMessage got unexpected diff: %s", diff)
			}
		})
	}
}
