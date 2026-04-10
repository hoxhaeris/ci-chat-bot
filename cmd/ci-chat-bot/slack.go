package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	jiraClient "github.com/andygrunwald/go-jira"
	"github.com/openshift/ci-chat-bot/pkg/jira"
	"github.com/openshift/ci-chat-bot/pkg/manager"
	"github.com/openshift/ci-chat-bot/pkg/slack"
	eventhandler "github.com/openshift/ci-chat-bot/pkg/slack/events"
	eventrouter "github.com/openshift/ci-chat-bot/pkg/slack/events/router"
	interactionhandler "github.com/openshift/ci-chat-bot/pkg/slack/interactions"
	interactionrouter "github.com/openshift/ci-chat-bot/pkg/slack/interactions/router"
	"github.com/sirupsen/logrus"
	slackClient "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	"sigs.k8s.io/prow/pkg/config"
	prowflagutil "sigs.k8s.io/prow/pkg/flagutil"
	"sigs.k8s.io/prow/pkg/interrupts"
	"sigs.k8s.io/prow/pkg/metrics"
	"sigs.k8s.io/prow/pkg/pjutil"
	"sigs.k8s.io/prow/pkg/pjutil/pprof"
	"sigs.k8s.io/prow/pkg/simplifypath"
)

func l(fragment string, children ...simplifypath.Node) simplifypath.Node {
	return simplifypath.L(fragment, children...)
}

func Start(bot *slack.Bot, jiraclient *jiraClient.Client, jobManager manager.JobManager, httpclient *http.Client, health *pjutil.Health, iOpts prowflagutil.InstrumentationOptions, clusterBotMetrics *metrics.Metrics) {
	slackclient := slackClient.New(bot.BotToken)
	jobManager.SetNotifier(bot.JobResponder(slackclient))
	jobManager.SetRosaNotifier(bot.RosaResponder(slackclient))
	jobManager.SetMceNotifier(bot.MceResponder(slackclient))
	var issueFiler jira.IssueFiler
	if jiraclient != nil {
		var err error
		issueFiler, err = jira.NewIssueFiler(slackclient, jiraclient)
		if err != nil {
			klog.Errorf(" Could not initialize Jira issue filer: %s", err)
		}
	} else {
		issueFiler = nil
	}

	metrics.ExposeMetrics("ci-chat-bot", config.PushGateway{}, iOpts.MetricsPort)
	simplifier := simplifypath.NewSimplifier(l("", // shadow element mimicking the root
		l(""),       // for black-box health checks
		l("readyz"), // for readyness probe check
		l("slack",
			l("events-endpoint"),
		),
	))
	handler := metrics.TraceHandler(simplifier, clusterBotMetrics.HTTPRequestDuration, clusterBotMetrics.HTTPResponseSize)
	pprof.Instrument(iOpts)
	mux := http.NewServeMux()
	// handle the root to allow for a simple uptime probe
	mux.Handle("/", handler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { writer.WriteHeader(http.StatusOK) })))
	mux.Handle("/readyz", handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))) // report ready once the server is up and responding
	mux.Handle("/slack/events-endpoint", handler(handleEvent(bot.BotSigningSecret, eventrouter.ForEvents(slackclient, jobManager, bot.SupportedCommands(), issueFiler, bot.AIClient))))
	mux.Handle("/slack/interactive-endpoint", handler(handleInteraction(bot.BotSigningSecret, interactionrouter.ForModals(slackclient, jobManager, httpclient, bot.AIClient))))
	mux.Handle("/api/v1/jobs/validate", handleJobValidation(jobManager))
	mux.Handle("/api/v1/jobs/supported", handleSupportedOptions(jobManager))
	mux.Handle("/api/v1/workflows/validate", handleWorkflowValidation(jobManager))
	mux.Handle("/api/v1/mce/versions", handleMceVersions(jobManager))
	mux.Handle("/api/v1/mce/info", handleMceInfo())
	mux.Handle("/api/v1/rosa/info", handleRosaInfo(jobManager))
	mux.Handle("/api/v1/hypershift/info", handleHypershiftInfo())
	mux.Handle("/api/v1/quota/status", handleQuotaStatus(jobManager))
	mux.Handle("/api/v1/capacity/status", handleCapacityStatus(jobManager))
	server := &http.Server{Addr: ":" + strconv.Itoa(bot.Port), Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	health.ServeReady(func() bool {
		resp, err := http.DefaultClient.Get("http://127.0.0.1:" + strconv.Itoa(bot.Port) + "/readyz")
		if resp != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				klog.Errorf("Failed to close response for readyz: %v", closeErr)
			}
		}
		return err == nil && resp.StatusCode == 200
	})

	interrupts.ListenAndServe(server, bot.GracePeriod)
	interrupts.WaitForGracefulShutdown()

	klog.Infof("ci-chat-bot up and listening to slack")
}

func handleEvent(signingSecret string, handler eventhandler.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := logrus.WithField("api", "events")
		body, ok := slack.VerifiedBody(request, signingSecret)
		if !ok {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		event, err := slackevents.ParseEvent(body, slackevents.OptionNoVerifyToken())
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		if event.Type == slackevents.URLVerification {
			var response *slackevents.ChallengeResponse
			err := json.Unmarshal(body, &response)
			if err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			writer.Header().Set("Content-Type", "text")
			if _, err := writer.Write([]byte(response.Challenge)); err != nil {
				klog.Errorf("Failed to write response. %v", err)
			}
		}

		// we always want to respond with 200 immediately
		writer.WriteHeader(http.StatusOK)
		// we don't really care how long this takes
		go func() {
			if err := handler.Handle(&event, logger); err != nil {
				klog.Errorf("Failed to handle event: %v", err)
			}
		}()
	}
}

func handleInteraction(signingSecret string, handler interactionhandler.Handler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		logger := logrus.WithField("api", "interactionhandler")
		if _, ok := slack.VerifiedBody(request, signingSecret); !ok {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		var callback slackClient.InteractionCallback
		payload := request.FormValue("payload")
		if err := json.Unmarshal([]byte(payload), &callback); err != nil {
			logger.WithError(err).WithField("payload", payload).Error("Failed to unmarshal an interaction payload.")
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		logger.WithField("interaction", callback).Trace("Read an interaction payload.")
		logger = logger.WithFields(fieldsFor(&callback))
		response, err := handler.Handle(&callback, logger)
		if err != nil {
			logger.WithError(err).Error("Failed to handle interaction payload.")
		}
		if len(response) == 0 {
			writer.WriteHeader(http.StatusOK)
			return
		}
		logger.WithField("body", string(response)).Trace("Sending interaction payload response.")
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Content-Length", strconv.Itoa(len(response)))
		if _, err := writer.Write(response); err != nil {
			logger.WithError(err).Error("Failed to send interaction payload response.")
		}
	}
}

func fieldsFor(interactionCallback *slackClient.InteractionCallback) logrus.Fields {
	return logrus.Fields{
		"trigger_id":  interactionCallback.TriggerID,
		"callback_id": interactionCallback.CallbackID,
		"action_id":   interactionCallback.ActionID,
		"type":        interactionCallback.Type,
	}
}

func handleJobValidation(jobManager manager.JobManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		platform := r.URL.Query().Get("platform")
		version := r.URL.Query().Get("version")
		if platform == "" || version == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"valid": false,
				"error": "platform and version are required query parameters",
			})
			return
		}

		arch := r.URL.Query().Get("arch")
		if arch == "" {
			arch = "amd64"
		}

		jobType := r.URL.Query().Get("type")
		if jobType == "" {
			jobType = "launch"
		}

		var jt manager.JobType
		switch jobType {
		case "launch":
			jt = manager.JobTypeInstall
		case "test":
			jt = manager.JobTypeTest
		case "upgrade":
			jt = manager.JobTypeUpgrade
		default:
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"valid": false,
				"error": "type must be one of: launch, test, upgrade",
			})
			return
		}

		params := make(map[string]string)
		if p := r.URL.Query().Get("params"); p != "" {
			for _, param := range strings.Split(p, ",") {
				param = strings.TrimSpace(param)
				if param != "" {
					params[param] = ""
				}
			}
		}

		req := manager.JobRequest{
			Inputs:       [][]string{{version}},
			Platform:     platform,
			Architecture: arch,
			Type:         jt,
			JobParams:    params,
		}

		err := jobManager.CheckValidJobConfiguration(&req)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"valid": false,
				"error": err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid": true,
		})
	}
}

func handleSupportedOptions(jobManager manager.JobManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		workflowConfig := jobManager.GetWorkflowConfig()
		workflowConfig.Mutex.RLock()
		workflows := make([]string, 0, len(workflowConfig.Workflows))
		for wf := range workflowConfig.Workflows {
			workflows = append(workflows, wf)
		}
		workflowConfig.Mutex.RUnlock()
		sort.Strings(workflows)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"platforms":     manager.SupportedPlatforms,
			"parameters":    manager.SupportedParameters,
			"architectures": manager.SupportedArchitectures,
			"tests":         manager.SupportedTests,
			"upgrade_tests": manager.SupportedUpgradeTests,
			"workflows":     workflows,
		})
	}
}

func handleWorkflowValidation(jobManager manager.JobManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		name := r.URL.Query().Get("name")
		if name == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"valid": false,
				"error": "name is a required query parameter",
			})
			return
		}

		workflowConfig := jobManager.GetWorkflowConfig()
		platform, architecture, err := slack.GetPlatformArchFromWorkflowConfig(workflowConfig, name)
		if err != nil {
			// Build structured list of available workflows
			workflowConfig.Mutex.RLock()
			available := make([]string, 0, len(workflowConfig.Workflows))
			for wf := range workflowConfig.Workflows {
				available = append(available, wf)
			}
			workflowConfig.Mutex.RUnlock()
			sort.Strings(available)

			json.NewEncoder(w).Encode(map[string]interface{}{
				"valid":               false,
				"error":               err.Error(),
				"available_workflows": available,
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":        true,
			"platform":     platform,
			"architecture": architecture,
		})
	}
}

func handleMceVersions(jobManager manager.JobManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		versions := jobManager.GetMceVersions()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"versions": versions,
			"count":    len(versions),
		})
	}
}

func handleMceInfo() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"platforms":                     manager.MCEPlatforms.UnsortedList(),
			"max_duration_hours":            int(manager.MaxMCEDuration.Hours()),
			"max_total_aws_clusters":        manager.MaxTotalMCEAWSClusters,
			"max_total_gcp_clusters":        manager.MaxTotalMCEGCPClusters,
			"default_max_clusters_per_user": 1,
		})
	}
}

func handleRosaInfo(jobManager manager.JobManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		versions := jobManager.GetRosaVersions()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"supported_versions":     versions,
			"max_duration_hours":     8,
			"default_duration_hours": 6,
			"commands":               []string{"rosa create <version> <duration>", "rosa lookup <version>", "rosa describe <cluster>"},
		})
	}
}

func handleHypershiftInfo() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		manager.HypershiftSupportedVersions.Mu.RLock()
		versions := sets.List(manager.HypershiftSupportedVersions.Versions)
		manager.HypershiftSupportedVersions.Mu.RUnlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"supported_versions": versions,
			"platforms":          []string{"hypershift-hosted", "hypershift-hosted-powervs"},
			"required_arch":      "multi",
		})
	}
}

func handleQuotaStatus(jobManager manager.JobManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobManager.GetQuotaStatus())
	}
}

func handleCapacityStatus(jobManager manager.JobManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobManager.GetCapacityStatus())
	}
}
