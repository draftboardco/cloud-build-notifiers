// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	cbpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/GoogleCloudPlatform/cloud-build-notifiers/lib/notifiers"
	log "github.com/golang/glog"
	"github.com/slack-go/slack"
)

const (
	webhookURLSecretName = "webhookUrl"
)

func main() {
	if err := notifiers.Main(new(slackNotifier)); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

type slackNotifier struct {
	filter     notifiers.EventFilter
	tmpl       *template.Template
	webhookURL string
	br         notifiers.BindingResolver
	tmplView   *notifiers.TemplateView
}

func (s *slackNotifier) SetUp(ctx context.Context, cfg *notifiers.Config, blockKitTemplate string, sg notifiers.SecretGetter, br notifiers.BindingResolver) error {
	prd, err := notifiers.MakeCELPredicate(cfg.Spec.Notification.Filter)
	if err != nil {
		return fmt.Errorf("failed to make a CEL predicate: %w", err)
	}
	s.filter = prd

	wuRef, err := notifiers.GetSecretRef(cfg.Spec.Notification.Delivery, webhookURLSecretName)
	if err != nil {
		return fmt.Errorf("failed to get Secret ref from delivery config (%v) field %q: %w", cfg.Spec.Notification.Delivery, webhookURLSecretName, err)
	}
	wuResource, err := notifiers.FindSecretResourceName(cfg.Spec.Secrets, wuRef)
	if err != nil {
		return fmt.Errorf("failed to find Secret for ref %q: %w", wuRef, err)
	}
	wu, err := sg.GetSecret(ctx, wuResource)
	if err != nil {
		return fmt.Errorf("failed to get token secret: %w", err)
	}
	s.webhookURL = wu
	tmpl, err := template.New("blockkit_template").Funcs(template.FuncMap{
		"replace": func(s, old, new string) string {
			return strings.ReplaceAll(s, old, new)
		},
		"getRepoName": func(source *cbpb.Source) string {
			if source == nil || source.Source == nil {
				return ""
			}
			switch s := source.Source.(type) {
			case *cbpb.Source_RepoSource:
				return s.RepoSource.RepoName
			case *cbpb.Source_GitSource:
				// Extract repo name from git URL
				url := s.GitSource.Url
				if url == "" {
					return ""
				}
				// Remove .git suffix if present
				url = strings.TrimSuffix(url, ".git")
				// Get the last part of the URL
				parts := strings.Split(url, "/")
				if len(parts) > 0 {
					return parts[len(parts)-2] + "/" + parts[len(parts)-1]
				}
				return ""
			case *cbpb.Source_StorageSource:
				return "gs://" + s.StorageSource.Bucket
			default:
				return ""
			}
		},
		"getSourceRef": func(source *cbpb.Source) string {
			if source == nil || source.Source == nil {
				return ""
			}
			switch s := source.Source.(type) {
			case *cbpb.Source_RepoSource:
				rs := s.RepoSource
				switch {
				case rs.GetBranchName() != "":
					return "branch/" + rs.GetBranchName()
				case rs.GetTagName() != "":
					return "tag/" + rs.GetTagName()
				case rs.GetCommitSha() != "":
					return "commit/" + rs.GetCommitSha()
				default:
					return ""
				}
			case *cbpb.Source_GitSource:
				if s.GitSource.Revision != "" {
					return fmt.Sprintf("commit/%s", s.GitSource.Revision)
				}
				return ""
			case *cbpb.Source_StorageSource:
				if s.StorageSource.Generation != 0 {
					return fmt.Sprintf("generation/%d", s.StorageSource.Generation)
				}
				return "object/" + s.StorageSource.Object
			default:
				return ""
			}
		},
		"getSourceType": func(source *cbpb.Source) string {
			if source == nil || source.Source == nil {
				return ""
			}
			switch source.Source.(type) {
			case *cbpb.Source_RepoSource:
				return "Cloud Source Repository"
			case *cbpb.Source_GitSource:
				return "Git Repository"
			case *cbpb.Source_StorageSource:
				return "Cloud Storage"
			default:
				return "Unknown"
			}
		},
		"getGitRef": func(build *cbpb.Build) string {
			if build == nil || build.Substitutions == nil {
				return ""
			}
			branch := build.Substitutions["BRANCH_NAME"]
			shortSha := build.Substitutions["SHORT_SHA"]
			if branch != "" && shortSha != "" {
				return fmt.Sprintf("branch/%s (%s)", branch, shortSha)
			}
			tag := build.Substitutions["TAG_NAME"]
			if tag != "" && shortSha != "" {
				return fmt.Sprintf("tag/%s (%s)", tag, shortSha)
			}
			if shortSha != "" {
				return fmt.Sprintf("commit/%s", shortSha)
			}
			return ""
		},
		"getDeploymentInfo": func(build *cbpb.Build) map[string]string {
			if build == nil || build.Substitutions == nil {
				return nil
			}
			info := make(map[string]string)

			// Try both standard and custom substitutions
			projectKeys := []string{"_CLUSTER_PROJECT", "PROJECT_ID"}
			for _, key := range projectKeys {
				if val := build.Substitutions[key]; val != "" {
					info["project"] = val
					break
				}
			}

			clusterKeys := []string{"_CLUSTER", "CLUSTER"}
			for _, key := range clusterKeys {
				if val := build.Substitutions[key]; val != "" {
					info["cluster"] = val
					break
				}
			}

			namespaceKeys := []string{"_NAMESPACE", "NAMESPACE"}
			for _, key := range namespaceKeys {
				if val := build.Substitutions[key]; val != "" {
					info["namespace"] = val
					break
				}
			}

			return info
		},
		"isProd": func(info map[string]string) bool {
			if info == nil {
				return false
			}
			// Check various production indicators in cluster name or namespace
			cluster := strings.ToLower(info["cluster"])
			namespace := strings.ToLower(info["namespace"])
			return strings.Contains(cluster, "prod") || strings.Contains(namespace, "prod") || namespace == "p" || namespace == "prd"
		},
	}).Parse(blockKitTemplate)

	s.tmpl = tmpl
	s.br = br

	return nil
}

func (s *slackNotifier) SendNotification(ctx context.Context, build *cbpb.Build) error {

	if !s.filter.Apply(ctx, build) {
		return nil
	}

	log.Infof("sending Slack webhook for Build %q (status: %q)", build.Id, build.Status)

	bindings, err := s.br.Resolve(ctx, nil, build)
	if err != nil {
		return fmt.Errorf("failed to resolve bindings: %w", err)
	}

	s.tmplView = &notifiers.TemplateView{
		Build:  &notifiers.BuildView{Build: build},
		Params: bindings,
	}

	msg, err := s.writeMessage()

	if err != nil {
		return fmt.Errorf("failed to write Slack message: %w", err)
	}

	return slack.PostWebhook(s.webhookURL, msg)
}

func (s *slackNotifier) writeMessage() (*slack.WebhookMessage, error) {
	build := s.tmplView.Build
	_, err := notifiers.AddUTMParams(build.LogUrl, notifiers.ChatMedium)

	if err != nil {
		return nil, fmt.Errorf("failed to add UTM params: %w", err)
	}

	var clr string
	switch build.Status {
	case cbpb.Build_SUCCESS:
		clr = "#22bb33"
	case cbpb.Build_FAILURE, cbpb.Build_INTERNAL_ERROR, cbpb.Build_TIMEOUT:
		clr = "#bb2124"
	default:
		clr = "#f0ad4e"
	}

	var buf bytes.Buffer
	if err := s.tmpl.Execute(&buf, s.tmplView); err != nil {
		return nil, err
	}
	var blocks slack.Blocks

	err = blocks.UnmarshalJSON(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal templating JSON: %w", err)
	}

	return &slack.WebhookMessage{Attachments: []slack.Attachment{{Color: clr, Blocks: blocks}}}, nil
}
