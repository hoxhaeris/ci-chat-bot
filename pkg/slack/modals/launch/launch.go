package launch

import (
	"encoding/json"
	"github.com/openshift/ci-chat-bot/pkg/manager"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"

	"github.com/openshift/ci-chat-bot/pkg/slack/interactions"
	"github.com/openshift/ci-chat-bot/pkg/slack/modals"
)

// Identifier is the view identifier for this modal
const Identifier modals.Identifier = "launch"
const Identifier2ndStep modals.Identifier = "launch2ndStep"
const Identifier3rdStep modals.Identifier = "launch3ddStep"

func RegisterFirstStep(client *slack.Client, jobmanager manager.JobManager) *modals.FlowWithViewAndFollowUps {
	return modals.ForView(Identifier, FirstStepView()).WithFollowUps(map[slack.InteractionType]interactions.Handler{
		slack.InteractionTypeViewSubmission: launchNextFirstStep(client, jobmanager),
	})
}

func launchNextFirstStep(updater modals.ViewUpdater, jobmanager manager.JobManager) interactions.Handler {
	return interactions.HandlerFunc("launch2", func(callback *slack.InteractionCallback, logger *logrus.Entry) (output []byte, err error) {
		go func() {
			overwriteView := func(view slack.ModalViewRequest) {
				// don't pass a hash so we overwrite the View always
				response, err := updater.UpdateView(view, "", "", callback.View.ID)
				if err != nil {
					logger.WithError(err).Warn("Failed to update a modal View.")
				}
				logger.WithField("response", response).Trace("Got a modal response.")
			}
			overwriteView(SecondStepView(callback, jobmanager))
		}()
		response, err := json.Marshal(&slack.ViewSubmissionResponse{
			ResponseAction: slack.RAUpdate,
			View:           PrepareNextStepView(),
		})
		if err != nil {
			logger.WithError(err).Error("Failed to marshal FirstStepView update submission response.")
			return nil, err
		}
		return response, nil
	})
}

func RegisterSecondStep(client *slack.Client, jobmanager manager.JobManager) *modals.FlowWithViewAndFollowUps {
	return modals.ForView(Identifier2ndStep, ThirdStepView(nil, jobmanager)).WithFollowUps(map[slack.InteractionType]interactions.Handler{
		slack.InteractionTypeViewSubmission: launchNextSecondStep(client, jobmanager),
	})
}

func launchNextSecondStep(updater modals.ViewUpdater, jobmanager manager.JobManager) interactions.Handler {
	return interactions.HandlerFunc("launch2", func(callback *slack.InteractionCallback, logger *logrus.Entry) (output []byte, err error) {
		go func() {
			overwriteView := func(view slack.ModalViewRequest) {
				// don't pass a hash so we overwrite the View always
				response, err := updater.UpdateView(view, "", "", callback.View.ID)
				if err != nil {
					logger.WithError(err).Warn("Failed to update a modal View.")
				}
				logger.WithField("response", response).Trace("Got a modal response.")
			}
			overwriteView(ThirdStepView(callback, jobmanager))
		}()
		response, err := json.Marshal(&slack.ViewSubmissionResponse{
			ResponseAction: slack.RAUpdate,
			View:           PrepareNextStepView(),
		})
		if err != nil {
			logger.WithError(err).Error("Failed to marshal FirstStepView update submission response.")
			return nil, err
		}
		return response, nil
	})
}
