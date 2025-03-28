

upload-template:
	gsutil cp ./slack/slack.json gs://cloud-build-configurations/slack/slack.json

reset-to-default:
	gsutil cp ./_misc/from-prod/slack.json gs://cloud-build-configurations/slack/slack.json

see-template:
	gsutil cat gs://cloud-build-configurations/slack/slack.json


#--------

upload-test:
	gsutil cp ./slack/slack.json gs://slack-build-notifier-config-draftboard-368620/slack/slack.json
reset-test-to-default:
	gsutil cp ./_misc/from-prod/slack-test.json gs://slack-build-notifier-config-draftboard-368620/slack/slack.json

see-test:
	gsutil cat gs://slack-build-notifier-config-draftboard-368620/slack/slack.json

upload-test-config:
	gsutil cp slack/slack-config.yaml gs://slack-build-notifier-config-draftboard-368620/slack/slack-config.yaml
see-test-config:
	gsutil cat gs://slack-build-notifier-config-draftboard-368620/slack/slack-config.yaml