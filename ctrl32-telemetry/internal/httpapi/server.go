// Package httpapi serves the REST API (URS G-01…) and the built-in pages. The UI is a client of the same API.
//
// Access model: sessions (cookie, CSRF header) or API tokens (Bearer). Read endpoints additionally allow
// anonymous access when http.public_read is enabled (demo instances only).
package httpapi

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/incident"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/timecheck"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/agent"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/console"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/escalate"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/flows"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/live"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/modbus"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/opcua"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/provision"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/push"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/replication"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/report"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/rules"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/smarthome"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/video"
)

//go:embed static/*
var static embed.FS

// Server holds dependencies for the handlers.
type Server struct {
	cfg              *config.Config
	pool             *pgxpool.Pool
	br               *broker.Broker
	iam              *iam.Service
	rep              *report.Engine
	ing              *ingest.Ingest
	con              *console.Manager
	log              *slog.Logger
	version          string
	agent            *agent.Client
	opc              *opcua.Manager
	started          time.Time
	loginLimiter     *limiter
	provisionLimiter *limiter
	pairLimiter      *limiter
	repl             *replication.Primary
	live             *live.Hub
	mb               *modbus.Manager
	esc              *escalate.Engine
	rules            *rules.Engine
	push             *push.Manager
	mux              *http.ServeMux
	tiles            *tileSource
	tc               *timecheck.Checker
	incidents        *incident.Manager
	flows            *flows.Engine
	prov             *provision.Service
	smart            *smarthome.Manager
	notifier         *notify.Notifier
	video            *video.Manager
	lic              licenseCache
}

// New builds the handler tree.
func New(cfg *config.Config, pool *pgxpool.Pool, br *broker.Broker, iamSvc *iam.Service, rep *report.Engine, ing *ingest.Ingest, con *console.Manager, ag *agent.Client, opc *opcua.Manager, log *slog.Logger, version string, hub *live.Hub, mbm *modbus.Manager, esc *escalate.Engine, eng *rules.Engine, pm *push.Manager, tc *timecheck.Checker, inc *incident.Manager, rp *replication.Primary, fl *flows.Engine, nt *notify.Notifier, sh *smarthome.Manager, vm *video.Manager) http.Handler {
	s := &Server{cfg: cfg, pool: pool, br: br, iam: iamSvc, rep: rep, ing: ing, con: con, agent: ag, opc: opc, started: time.Now(), log: log.With("component", "http"), version: version, live: hub, mb: mbm, esc: esc, rules: eng, push: pm, tc: tc, incidents: inc, repl: rp, flows: fl, notifier: nt, smart: sh, video: vm,
		loginLimiter: newLimiter(10, 10), provisionLimiter: newLimiter(20, 10), pairLimiter: newLimiter(5, 5)}
	if eng != nil {
		eng.Commander = s
	}
	if fl != nil {
		fl.SetHost(newFlowHost(s))
	}
	s.prov = provision.New(pool, s.log)
	if br != nil {
		br.OnProvision = s.prov.HandleMQTT
	}
	mux := http.NewServeMux()
	s.mux = mux
	s.tiles = openTiles(cfg.Map, s.log)
	if pool != nil {
		go s.licenseRenewLoop() // daily renewal of an activated license (license_activation.go)
	}
	sub, _ := fs.Sub(static, "static")
	files := http.FileServer(http.FS(sub))
	if rep != nil {
		rep.Product = s.reportProduct
	}

	// public
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/mfa", s.loginMFA)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("POST /api/v1/provision", s.provision)
	mux.HandleFunc("POST /api/v1/auth/forgot", s.forgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset", s.resetPasswordByLink)
	mux.HandleFunc("POST /api/v1/auth/register-request", s.registerRequest)
	mux.HandleFunc("POST /api/v1/ingest/chirpstack/{id}", s.chirpstackIngest)
	mux.HandleFunc("GET /api/v1/d/fota/{fw}/blob", s.firmwareBlob)
	mux.HandleFunc("POST /api/v1/d/telemetry", s.devTelemetry)
	mux.HandleFunc("POST /api/v1/d/attributes", s.devAttributes)
	mux.HandleFunc("POST /api/v1/d/status", s.devStatus)
	mux.HandleFunc("GET /api/v1/d/attributes/shared", s.devSharedAttributes)
	mux.HandleFunc("GET /api/v1/d/cmd", s.devCommands)
	mux.HandleFunc("POST /api/v1/d/cmd/{id}/ack", s.devCommandAck)
	mux.HandleFunc("POST /api/v1/d/fota/status", s.devFotaStatus)
	mux.HandleFunc("GET /api/v1/branding", s.getBranding)
	mux.HandleFunc("POST /api/v1/replication/pair", s.pairReplica)
	mux.HandleFunc("POST /api/v1/flow-hooks/{token}", s.flowHook)
	mux.HandleFunc("GET /api/v1/replication/tunnel", s.replicationTunnel)
	mux.HandleFunc("GET /api/v1/replication/agent", s.replicationAgentPoll)
	mux.HandleFunc("POST /api/v1/replication/agent", s.replicationAgentReport)
	mux.HandleFunc("GET /api/v1/replication/role", s.replicationRole)
	mux.HandleFunc("GET /api/v1/openapi.json", s.openapiJSON)
	mux.HandleFunc("GET /api/v1/branding/logo", s.getBrandingLogo)
	mux.HandleFunc("GET /api/v1/landing", s.publicLanding)
	mux.HandleFunc("GET /api/v1/landing/assets/{id}", s.landingAsset)
	mux.HandleFunc("GET /api/v1/tiles", s.tilesInfo)
	mux.HandleFunc("GET /tiles/{z}/{x}/{y}", s.tile)
	mux.HandleFunc("GET /.well-known/security.txt", s.securityTxt)
	mux.HandleFunc("GET /security.txt", s.securityTxtRedirect)
	mux.Handle("GET /metrics", s.require("data.read", s.metricsHandler))
	mux.HandleFunc("GET /api/v1/i18n/languages", s.getLanguages)
	mux.HandleFunc("GET /api/v1/i18n/{locale}", s.getCatalog)
	mux.HandleFunc("GET /license", s.licensePage)
	mux.HandleFunc("GET /sw.js", s.serviceWorker)
	mux.HandleFunc("GET /manifest.webmanifest", s.manifest)
	mux.HandleFunc("GET /api/v1/branding/icon", s.getBrandingIcon)

	// authenticated (any permission)
	mux.Handle("GET /api/v1/auth/me", s.require("", s.me))
	mux.Handle("POST /api/v1/auth/password", s.require("", s.changePassword))
	mux.Handle("POST /api/v1/auth/totp/setup", s.require("", s.totpSetup))
	mux.Handle("POST /api/v1/auth/totp/confirm", s.require("", s.totpConfirm))

	// data
	mux.Handle("GET /api/v1/datastreams", s.require("data.read", s.datastreams))
	mux.Handle("GET /api/v1/devices", s.require("data.read", s.devices))
	mux.Handle("GET /api/v1/observations", s.require("data.read", s.observations))
	mux.Handle("GET /api/v1/datastreams/latest", s.require("data.read", s.latestValues))
	mux.Handle("GET /api/v1/dashboards", s.require("data.read", s.listDashboards))
	mux.Handle("GET /api/v1/dashboards/catalog", s.require("data.read", s.dashboardCatalog))
	mux.Handle("GET /api/v1/dashboards/starter", s.require("data.read", s.dashboardStarter))
	mux.Handle("GET /api/v1/dashboards/{slug}/share", s.require("content.write", s.getShare))
	mux.Handle("POST /api/v1/dashboards/{slug}/share", s.require("content.write", s.createShare))
	mux.Handle("DELETE /api/v1/dashboards/{slug}/share", s.require("content.write", s.deleteShare))
	// public read-only dashboards: no session; the token decides what is visible (public_dashboard.go)
	// displays (kiosks): no session; the display cookie decides what is visible (displays.go)
	mux.HandleFunc("POST /api/v1/display/hello", s.displayHello)
	mux.HandleFunc("GET /api/v1/display/state", s.displayState)
	mux.HandleFunc("GET /api/v1/display/walls/{id}", s.displayWall)
	mux.HandleFunc("GET /api/v1/display/cameras/{id}/live", s.displayLive)
	mux.HandleFunc("GET /api/v1/public/{token}", s.publicDashboard)
	mux.HandleFunc("GET /api/v1/public/{token}/datastreams", s.publicDatastreams)
	mux.HandleFunc("GET /api/v1/public/{token}/datastreams/latest", s.publicLatest)
	mux.HandleFunc("GET /api/v1/public/{token}/observations", s.publicObservations)
	mux.HandleFunc("GET /api/v1/public/{token}/devices", s.publicDevices)
	mux.HandleFunc("GET /api/v1/public/{token}/alarms", s.publicAlarms)
	mux.HandleFunc("GET /api/v1/public/{token}/stream", s.publicStream)
	mux.HandleFunc("GET /api/v1/public/{token}/hmi/objects/{slug}/svg", s.publicHMIObjectSVG)
	mux.Handle("GET /api/v1/stream", s.require("data.read", s.stream))
	mux.Handle("GET /api/v1/dashboards/{slug}", s.require("data.read", s.getDashboard))
	mux.Handle("POST /api/v1/dashboards", s.require("content.write", s.createDashboard))
	mux.Handle("PUT /api/v1/dashboards/{slug}", s.require("content.write", s.updateDashboard))
	mux.Handle("DELETE /api/v1/dashboards/{slug}", s.require("content.write", s.deleteDashboard))
	// HMI object library for process pictures: imported SVG graphics and composites (hmi_objects.go)
	mux.Handle("GET /api/v1/hmi/objects", s.require("data.read", s.listHMIObjects))
	mux.Handle("POST /api/v1/hmi/objects", s.require("content.write", s.createHMIObject))
	mux.Handle("POST /api/v1/hmi/objects/import-svg", s.require("content.write", s.importSVG))
	mux.Handle("GET /api/v1/hmi/objects/{slug}", s.require("data.read", s.getHMIObject))
	mux.Handle("PUT /api/v1/hmi/objects/{slug}", s.require("content.write", s.updateHMIObject))
	mux.Handle("DELETE /api/v1/hmi/objects/{slug}", s.require("content.write", s.deleteHMIObject))
	mux.Handle("GET /api/v1/hmi/objects/{slug}/svg", s.require("data.read", s.hmiObjectSVG))
	mux.Handle("GET /api/v1/alarms", s.require("data.read", s.alarms))
	mux.Handle("GET /api/v1/incidents", s.require("data.read", s.listIncidents))
	mux.Handle("GET /api/v1/incidents/{id}", s.require("data.read", s.getIncident))
	mux.Handle("POST /api/v1/incidents", s.require("alarm.ack", s.createIncident))
	mux.Handle("PATCH /api/v1/incidents/{id}", s.require("alarm.ack", s.updateIncident))
	mux.Handle("GET /api/v1/audit/verify", s.require("data.read", s.auditVerify))
	mux.Handle("POST /api/v1/alarms/{id}/ack", s.require("alarm.ack", s.ackAlarm))

	// devices
	mux.Handle("POST /api/v1/devices", s.require("device.manage", s.createDevice))
	mux.Handle("POST /api/v1/devices/{id}/token", s.require("device.manage", s.rotateDeviceToken))
	mux.Handle("POST /api/v1/devices/{id}/revoke", s.require("device.manage", s.revokeDevice))
	mux.Handle("GET /api/v1/devices/{id}/messages", s.require("data.read", s.deviceMessages))
	mux.Handle("GET /api/v1/devices/{id}", s.require("data.read", s.getDevice))
	mux.Handle("PATCH /api/v1/devices/{id}", s.require("device.manage", s.patchDevice))
	mux.Handle("POST /api/v1/devices/{id}/datastreams", s.require("device.manage", s.fourEyes("assignments", s.assignDatastream)))
	mux.Handle("DELETE /api/v1/devices/{id}/datastreams/{ds}", s.require("device.manage", s.fourEyes("assignments", s.unassignDatastream)))
	mux.Handle("GET /api/v1/devices/{id}/commands", s.require("data.read", s.listCommands))
	mux.Handle("POST /api/v1/devices/{id}/commands", s.require("device.command", s.sendCommand))
	mux.Handle("POST /api/v1/devices/{id}/commands/{cid}/approve", s.require("device.command", s.approveCommand))
	mux.Handle("POST /api/v1/devices/{id}/commands/{cid}/reject", s.require("device.command", s.rejectCommand))
	mux.Handle("GET /api/v1/settings/policy", s.require("config.write", s.getPolicy))
	mux.Handle("GET /api/v1/notification-channels", s.require("config.write", s.listChannels))
	mux.Handle("POST /api/v1/notification-channels", s.require("config.write", s.createChannel))
	mux.Handle("PUT /api/v1/notification-channels/{id}", s.require("config.write", s.updateChannel))
	mux.Handle("DELETE /api/v1/notification-channels/{id}", s.require("config.write", s.deleteChannel))
	mux.Handle("POST /api/v1/notification-channels/{id}/test", s.require("config.write", s.testChannel))
	mux.Handle("GET /api/v1/escalation-policies", s.require("config.write", s.listPolicies))
	mux.Handle("POST /api/v1/escalation-policies", s.require("config.write", s.createPolicy))
	mux.Handle("PUT /api/v1/escalation-policies/{id}", s.require("config.write", s.updatePolicy))
	mux.Handle("DELETE /api/v1/escalation-policies/{id}", s.require("config.write", s.deletePolicy))
	mux.Handle("GET /api/v1/notifications", s.require("config.write", s.listNotifications))
	mux.Handle("GET /api/v1/rules", s.require("config.write", s.listRules))
	mux.Handle("POST /api/v1/rules", s.require("config.write", s.createRule))
	mux.Handle("POST /api/v1/rules/test", s.require("config.write", s.testRule))
	mux.Handle("PUT /api/v1/rules/{id}", s.require("config.write", s.updateRule))
	mux.Handle("DELETE /api/v1/rules/{id}", s.require("config.write", s.deleteRule))
	mux.Handle("GET /api/v1/rules/{id}/as-flow", s.require("config.write", s.ruleAsFlow))
	mux.Handle("POST /api/v1/rules/{id}/approve", s.require("device.command", s.approveRule))
	mux.Handle("POST /api/v1/rules/{id}/reject", s.require("device.command", s.rejectRule))
	mux.Handle("GET /api/v1/rule-runs", s.require("config.write", s.listRuleRuns))
	mux.Handle("PUT /api/v1/settings/policy", s.require("user.admin", s.putPolicy))
	mux.Handle("GET /api/v1/devices/{id}/attributes", s.require("data.read", s.getAttributes))
	mux.Handle("GET /api/v1/provision-profiles", s.require("device.manage", s.listProvProfiles))
	mux.Handle("POST /api/v1/provision-profiles", s.require("device.manage", s.createProvProfile))
	mux.Handle("PUT /api/v1/provision-profiles/{id}", s.require("device.manage", s.updateProvProfile))
	mux.Handle("POST /api/v1/provision-profiles/{id}/rotate", s.require("device.manage", s.rotateProvProfile))
	mux.Handle("DELETE /api/v1/provision-profiles/{id}", s.require("device.manage", s.revokeProvProfile))
	mux.Handle("GET /api/v1/device-approvals", s.require("device.manage", s.listApprovals))
	mux.Handle("GET /api/v1/mqtt-sources", s.require("device.manage", s.listMQTTSources))
	mux.Handle("POST /api/v1/mqtt-sources", s.require("device.manage", s.createMQTTSource))
	mux.Handle("PUT /api/v1/mqtt-sources/{id}", s.require("device.manage", s.updateMQTTSource))
	mux.Handle("POST /api/v1/mqtt-sources/{id}/password", s.require("device.manage", s.mqttAccountPassword))
	mux.Handle("DELETE /api/v1/mqtt-sources/{id}", s.require("device.manage", s.deleteMQTTSource))
	mux.Handle("GET /api/v1/discovery", s.require("device.manage", s.listDiscovery))
	mux.Handle("POST /api/v1/discovery/{id}/adopt", s.require("device.manage", s.adoptDiscovery))
	mux.Handle("POST /api/v1/discovery/{id}/ignore", s.require("device.manage", s.ignoreDiscovery))
	mux.Handle("GET /api/v1/entities", s.require("data.read", s.listEntities))
	mux.Handle("GET /api/v1/entities/{id}", s.require("data.read", s.getEntity))
	mux.Handle("PATCH /api/v1/entities/{id}", s.require("device.manage", s.patchEntity))
	mux.Handle("POST /api/v1/entities/{id}/command", s.require("device.command", s.entityCommand))
	mux.Handle("GET /api/v1/home", s.require("data.read", s.home))
	mux.Handle("GET /api/v1/scenes", s.require("data.read", s.listScenes))
	mux.Handle("POST /api/v1/scenes", s.require("device.manage", s.createScene))
	mux.Handle("PUT /api/v1/scenes/{id}", s.require("device.manage", s.updateScene))
	mux.Handle("DELETE /api/v1/scenes/{id}", s.require("device.manage", s.deleteScene))
	mux.Handle("POST /api/v1/scenes/{id}/activate", s.require("device.command", s.activateScene))
	// the public front page (landing.go)
	mux.Handle("GET /api/v1/landing/draft", s.require("system.admin", s.landingDraft))
	mux.Handle("GET /api/v1/landing/versions", s.require("system.admin", s.landingVersions))
	mux.Handle("GET /api/v1/landing/versions/{v}", s.require("system.admin", s.landingVersion))
	mux.Handle("POST /api/v1/landing/versions", s.require("system.admin", s.saveLanding))
	mux.Handle("POST /api/v1/landing/versions/{v}/publish", s.require("system.admin", s.publishLanding))
	mux.Handle("POST /api/v1/landing/unpublish", s.require("system.admin", s.unpublishLanding))
	mux.Handle("GET /api/v1/landing/assets", s.require("system.admin", s.landingAssets))
	mux.Handle("POST /api/v1/landing/assets", s.require("system.admin", s.uploadLandingAsset))
	// cameras, recordings and video walls (video.go)
	mux.Handle("GET /api/v1/video/cameras", s.require("video.view", s.listCameras))
	mux.Handle("POST /api/v1/video/cameras", s.require("video.manage", s.createCamera))
	mux.Handle("POST /api/v1/video/cameras/test", s.require("video.manage", s.testCamera))
	mux.Handle("GET /api/v1/video/cameras/{id}", s.require("video.view", s.getCamera))
	mux.Handle("PUT /api/v1/video/cameras/{id}", s.require("video.manage", s.updateCamera))
	mux.Handle("POST /api/v1/video/cameras/{id}/push-secret", s.require("video.manage", s.rotatePushSecret))
	mux.Handle("DELETE /api/v1/video/cameras/{id}", s.require("video.manage", s.deleteCamera))
	mux.Handle("GET /api/v1/video/cameras/{id}/live", s.require("video.view", s.cameraLive))
	mux.Handle("GET /api/v1/video/cameras/{id}/snapshot", s.require("video.view", s.cameraSnapshot))
	mux.Handle("GET /api/v1/video/cameras/{id}/recordings", s.require("video.playback", s.cameraRecordings))
	mux.Handle("GET /api/v1/video/cameras/{id}/playback", s.require("video.playback", s.cameraPlayback))
	mux.Handle("GET /api/v1/video/cameras/{id}/export", s.require("video.playback", s.cameraExport))
	mux.Handle("GET /api/v1/video/storage", s.require("video.manage", s.videoStorage))
	mux.Handle("POST /api/v1/video/discover", s.require("video.manage", s.videoDiscover))
	mux.Handle("POST /api/v1/video/onvif/profiles", s.require("video.manage", s.onvifProfiles))
	mux.Handle("GET /api/v1/video/events", s.require("video.playback", s.cameraEvents))
	mux.Handle("GET /api/v1/video/cameras/{id}/events", s.require("video.playback", s.cameraEvents))
	mux.Handle("POST /api/v1/video/cameras/{id}/events", s.require("video.view", s.markCamera))
	mux.Handle("GET /api/v1/video/cameras/{id}/ptz/presets", s.require("video.ptz", s.ptzPresets))
	mux.Handle("POST /api/v1/video/cameras/{id}/ptz", s.require("video.ptz", s.cameraPTZ))
	// camera control through the relay: the relay authenticates with the camera's secret (video_relay.go)
	mux.HandleFunc("GET /api/v1/video/relay/{id}/onvif/next", s.relayONVIFNext)
	mux.HandleFunc("POST /api/v1/video/relay/{id}/onvif/{rid}", s.relayONVIFAnswer)
	mux.Handle("GET /api/v1/displays", s.require("display.control", s.listDisplays))
	mux.Handle("POST /api/v1/displays/pair", s.require("display.manage", s.pairDisplay))
	mux.Handle("PUT /api/v1/displays/{id}", s.require("display.manage", s.updateDisplay))
	mux.Handle("DELETE /api/v1/displays/{id}", s.require("display.manage", s.revokeDisplay))
	mux.Handle("POST /api/v1/displays/{id}/command", s.require("display.control", s.commandDisplay))
	mux.Handle("POST /api/v1/displays/command", s.require("display.control", s.commandDisplays))
	mux.Handle("GET /api/v1/video/holds", s.require("video.playback", s.listHolds))
	mux.Handle("POST /api/v1/video/holds", s.require("video.evidence", s.createHold))
	mux.Handle("DELETE /api/v1/video/holds/{id}", s.require("video.evidence", s.releaseHold))
	mux.Handle("GET /api/v1/video/cameras/{id}/evidence", s.require("video.evidence", s.cameraEvidence))
	mux.Handle("GET /api/v1/video/evidence/key", s.require("video.playback", s.evidenceKey))
	mux.Handle("GET /api/v1/video/camera-groups", s.require("video.view", s.cameraGroups))
	mux.Handle("GET /api/v1/video/settings", s.require("video.manage", s.videoSettings))
	mux.Handle("PUT /api/v1/video/settings", s.require("system.admin", s.putVideoSettings))
	mux.Handle("GET /api/v1/video/walls", s.require("video.view", s.listWalls))
	mux.Handle("POST /api/v1/video/walls", s.require("video.manage", s.saveWall))
	mux.Handle("GET /api/v1/video/walls/{id}", s.require("video.view", s.getWall))
	mux.Handle("PUT /api/v1/video/walls/{id}", s.require("video.manage", s.saveWall))
	mux.Handle("DELETE /api/v1/video/walls/{id}", s.require("video.manage", s.deleteWall))
	mux.Handle("GET /api/v1/energy", s.require("data.read", s.energy))
	mux.Handle("GET /api/v1/energy/settings", s.require("data.read", s.getEnergySettings))
	mux.Handle("PUT /api/v1/energy/settings", s.require("content.write", s.putEnergySettings))
	mux.Handle("GET /api/v1/onboarding", s.require("device.manage", s.onboarding))
	mux.Handle("POST /api/v1/devices/{id}/approve", s.require("device.manage", s.decideDevice(true)))
	mux.Handle("POST /api/v1/devices/{id}/reject", s.require("device.manage", s.decideDevice(false)))
	mux.Handle("PUT /api/v1/devices/{id}/attributes", s.require("device.manage", s.putAttributes))
	mux.Handle("GET /api/v1/devices/{id}/console", s.require("device.console", s.consoleStatus))
	mux.Handle("POST /api/v1/devices/{id}/console", s.require("device.console", s.consoleOpen))
	mux.Handle("DELETE /api/v1/devices/{id}/console/{sid}", s.require("device.console", s.consoleClose))
	mux.Handle("POST /api/v1/devices/{id}/console/{sid}/in", s.require("device.console", s.consoleSend))
	mux.Handle("GET /api/v1/devices/{id}/console/{sid}", s.require("device.console", s.consoleLines))
	mux.Handle("GET /api/v1/firmware", s.require("fota.read", s.listFirmware))
	mux.Handle("POST /api/v1/firmware", s.require("fota.manage", s.uploadFirmware))
	mux.Handle("GET /api/v1/fota/campaigns", s.require("fota.read", s.listCampaigns))
	mux.Handle("POST /api/v1/fota/campaigns", s.require("fota.manage", s.createCampaign))
	mux.Handle("GET /api/v1/fota/campaigns/{id}", s.require("fota.read", s.getCampaign))
	mux.Handle("DELETE /api/v1/fota/campaigns/{id}", s.require("fota.manage", s.cancelCampaign))
	mux.Handle("GET /api/v1/connectors", s.require("config.write", s.listConnectors))
	mux.Handle("POST /api/v1/connectors", s.require("config.write", s.createConnector))
	mux.Handle("DELETE /api/v1/connectors/{id}", s.require("config.write", s.revokeConnector))
	mux.Handle("PUT /api/v1/connectors/{id}", s.require("config.write", s.updateConnector))
	mux.Handle("POST /api/v1/connectors/test", s.require("config.write", s.testConnector))
	mux.Handle("GET /api/v1/connectors/opcua/certificate", s.require("config.write", s.opcuaCertificate))
	mux.Handle("GET /api/v1/register-profiles", s.require("config.write", s.listProfiles))
	mux.Handle("POST /api/v1/register-profiles", s.require("config.write", s.createProfile))
	mux.Handle("GET /api/v1/register-profiles/{id}", s.require("config.write", s.getProfile))
	mux.Handle("PUT /api/v1/register-profiles/{id}", s.require("config.write", s.updateProfile))
	mux.Handle("DELETE /api/v1/register-profiles/{id}", s.require("config.write", s.deleteProfile))
	mux.Handle("POST /api/v1/register-profiles/{id}/apply", s.require("config.write", s.applyProfile))
	mux.Handle("GET /api/v1/export/organization", s.require("system.admin", s.exportOrganization))
	mux.Handle("GET /api/v1/flows/catalog", s.require("data.read", s.flowCatalog))
	mux.Handle("GET /api/v1/flows/settings", s.require("data.read", s.flowSettings))
	mux.Handle("PUT /api/v1/flows/settings", s.require("config.write", s.flowSettings))
	mux.Handle("POST /api/v1/flows/validate", s.require("data.read", s.validateFlow))
	mux.Handle("POST /api/v1/flows/import", s.require("content.write", s.importFlow))
	mux.Handle("GET /api/v1/flows", s.require("data.read", s.listFlows))
	mux.Handle("POST /api/v1/flows", s.require("content.write", s.createFlow))
	mux.Handle("GET /api/v1/flows/{slug}", s.require("data.read", s.getFlow))
	mux.Handle("PUT /api/v1/flows/{slug}", s.require("content.write", s.updateFlow))
	mux.Handle("DELETE /api/v1/flows/{slug}", s.require("content.write", s.deleteFlow))
	mux.Handle("POST /api/v1/flows/{slug}/deploy", s.require("content.write", s.fourEyes("flows", s.deployFlow)))
	mux.Handle("POST /api/v1/flows/{slug}/disable", s.require("content.write", s.disableFlow))
	mux.Handle("GET /api/v1/flows/{slug}/versions", s.require("data.read", s.flowVersions))
	mux.Handle("POST /api/v1/flows/{slug}/restore", s.require("content.write", s.restoreFlow))
	mux.Handle("POST /api/v1/flows/{slug}/inject", s.require("content.write", s.injectFlow))
	mux.Handle("GET /api/v1/flows/{slug}/debug", s.require("data.read", s.flowDebug))
	mux.Handle("GET /api/v1/flows/{slug}/status", s.require("data.read", s.flowStatus))
	mux.Handle("GET /api/v1/flows/{slug}/export", s.require("data.read", s.exportFlow))
	mux.Handle("GET /api/v1/replication", s.require("system.admin", s.replicationStatus))
	mux.Handle("POST /api/v1/replication/pairings", s.require("system.admin", s.createPairing))
	mux.Handle("PATCH /api/v1/replication/replicas/{id}", s.require("system.admin", s.patchReplica))
	mux.Handle("POST /api/v1/replication/replicas/{id}/detach", s.require("system.admin", s.detachReplica))
	mux.Handle("DELETE /api/v1/replication/replicas/{id}", s.require("system.admin", s.dropReplica))
	mux.Handle("GET /api/v1/replication/replicas/{id}/events", s.require("system.admin", s.replicaEvents))
	mux.Handle("GET /api/v1/organizations", s.require("system.admin", s.listOrganizations))
	mux.Handle("POST /api/v1/organizations", s.require("system.admin", s.createOrganization))
	mux.Handle("PATCH /api/v1/organizations/{id}", s.require("system.admin", s.patchOrganization))
	mux.Handle("POST /api/v1/organizations/{id}/reset-admin", s.require("system.admin", s.resetOrgAdmin))
	mux.Handle("GET /api/v1/server", s.require("system.admin", s.serverInfo))
	mux.Handle("GET /api/v1/server/license", s.require("system.admin", s.getLicense))
	mux.Handle("PUT /api/v1/server/license", s.require("system.admin", s.putLicense))
	mux.Handle("DELETE /api/v1/server/license", s.require("system.admin", s.deleteLicense))
	mux.Handle("POST /api/v1/server/license/activate", s.require("system.admin", s.activateLicense))
	mux.Handle("POST /api/v1/server/license/renew", s.require("system.admin", s.renewLicenseNow))
	mux.Handle("POST /api/v1/server/restart", s.require("system.admin", s.serverRestart))
	mux.Handle("PUT /api/v1/server/caddy", s.require("system.admin", s.serverCaddy))
	mux.Handle("GET /api/v1/server/wireguard", s.require("system.admin", s.serverWireGuard))
	mux.Handle("PUT /api/v1/server/wireguard", s.require("system.admin", s.serverSetWireGuard))
	mux.Handle("POST /api/v1/server/wireguard/peers", s.require("system.admin", s.serverAddPeer))
	mux.Handle("POST /api/v1/server/wireguard/peers/{id}/remove", s.require("system.admin", s.serverRemovePeer))
	mux.Handle("POST /api/v1/server/update", s.require("system.admin", s.serverUpdate))
	mux.Handle("GET /api/v1/server/backups", s.require("system.admin", s.serverBackups))
	mux.Handle("POST /api/v1/server/backups", s.require("system.admin", s.serverBackupCreate))
	mux.Handle("PUT /api/v1/server/backups/schedule", s.require("system.admin", s.serverBackupSchedule))
	mux.Handle("GET /api/v1/server/backups/{name}", s.require("system.admin", s.serverBackupDownload))
	mux.Handle("POST /api/v1/server/backups/{name}/delete", s.require("system.admin", s.serverBackupDelete))

	// data model configuration (config.write)
	mux.Handle("GET /api/v1/sites", s.require("data.read", s.listSites))
	mux.Handle("POST /api/v1/sites", s.require("config.write", s.createSite))
	mux.Handle("PATCH /api/v1/sites/{id}", s.require("config.write", s.patchSite))
	mux.Handle("GET /api/v1/assets", s.require("data.read", s.listAssets))
	mux.Handle("POST /api/v1/assets", s.require("config.write", s.createAsset))
	mux.Handle("PATCH /api/v1/assets/{id}", s.require("config.write", s.patchAsset))
	mux.Handle("GET /api/v1/datastreams/{id}", s.require("data.read", s.getDatastream))
	mux.Handle("POST /api/v1/datastreams", s.require("config.write", s.createDatastream))
	mux.Handle("PATCH /api/v1/datastreams/{id}", s.require("config.write", s.patchDatastream))
	mux.Handle("GET /api/v1/alarm-rules", s.require("data.read", s.listAlarmRules))
	mux.Handle("POST /api/v1/alarm-rules", s.require("config.write", s.fourEyes("limits", s.createAlarmRule)))
	mux.Handle("DELETE /api/v1/alarm-rules/{id}", s.require("config.write", s.fourEyes("limits", s.closeAlarmRule)))
	mux.Handle("GET /api/v1/change-requests", s.require("", s.listChangeRequests))
	mux.Handle("POST /api/v1/change-requests/{id}/approve", s.require("", s.approveChange))
	mux.Handle("POST /api/v1/change-requests/{id}/reject", s.require("", s.rejectChange))

	// users and tokens
	mux.Handle("GET /api/v1/users", s.require("user.admin", s.listUsers))
	mux.Handle("POST /api/v1/users", s.require("user.admin", s.createUser))
	mux.Handle("PATCH /api/v1/users/{id}", s.require("user.admin", s.patchUser))
	mux.Handle("POST /api/v1/users/{id}/reset-password", s.require("user.admin", s.resetPassword))
	mux.Handle("GET /api/v1/users/{id}", s.require("user.admin", s.getUser))
	mux.Handle("GET /api/v1/users/{id}/camera-groups", s.require("user.admin", s.userCameraGroups))
	mux.Handle("PUT /api/v1/users/{id}/camera-groups", s.require("user.admin", s.putUserCameraGroups))
	mux.Handle("POST /api/v1/users/{id}/mfa-reset", s.require("user.admin", s.resetUserMFA()))
	mux.Handle("POST /api/v1/users/{id}/sessions/revoke", s.require("user.admin", s.revokeUserSessions()))
	mux.Handle("POST /api/v1/users/{id}/unlock", s.require("user.admin", s.unlockUser()))
	mux.Handle("GET /api/v1/permissions", s.require("", s.listPermissions))
	mux.Handle("GET /api/v1/roles", s.require("", s.listRoles))
	mux.Handle("POST /api/v1/roles", s.require("user.admin", s.createRole))
	mux.Handle("PUT /api/v1/roles/{id}", s.require("user.admin", s.updateRole))
	mux.Handle("GET /api/v1/account-requests", s.require("user.admin", s.listAccountRequests))
	mux.Handle("POST /api/v1/account-requests/{id}/dismiss", s.require("user.admin", s.handleAccountRequest("dismissed")))
	mux.Handle("POST /api/v1/account-requests/{id}/created", s.require("user.admin", s.handleAccountRequest("created")))
	mux.Handle("GET /api/v1/tokens", s.require("token.manage", s.listTokens))
	mux.Handle("POST /api/v1/tokens", s.require("token.manage", s.createToken))
	mux.Handle("DELETE /api/v1/tokens/{id}", s.require("token.manage", s.revokeToken))
	mux.Handle("PUT /api/v1/branding", s.require("content.write", s.putBranding))
	mux.Handle("GET /api/v1/branding/white-label", s.require("content.write", s.getWhiteLabel))
	mux.Handle("PUT /api/v1/i18n/languages", s.require("content.write", s.putLanguages))
	mux.Handle("GET /api/v1/i18n/{locale}/edit", s.require("content.write", s.getCatalogEdit))
	mux.Handle("PUT /api/v1/i18n/{locale}", s.require("content.write", s.putCatalog))
	mux.Handle("PUT /api/v1/auth/locale", s.require("", s.putUserLocale))
	mux.Handle("GET /api/v1/push/key", s.require("", s.pushKey))
	mux.Handle("GET /api/v1/push/subscriptions", s.require("", s.listPushSubscriptions))
	mux.Handle("POST /api/v1/push/subscriptions", s.require("", s.createPushSubscription))
	mux.Handle("DELETE /api/v1/push/subscriptions/{id}", s.require("", s.deletePushSubscription))
	mux.Handle("POST /api/v1/push/test", s.require("", s.pushTest))

	// exports
	mux.Handle("GET /api/v1/export/observations", s.require("data.export", s.exportObservations))
	mux.Handle("GET /api/v1/export/alarms", s.require("data.export", s.exportAlarms))
	mux.Handle("GET /api/v1/export/messages", s.require("data.export", s.exportMessages))
	mux.Handle("GET /api/v1/export/audit", s.require("audit.read", s.exportAudit))
	mux.Handle("GET /api/v1/export/access-log", s.require("audit.read", s.exportAccessLog))

	// report templates (content, ADR-0010) and stored reports (records)
	mux.Handle("GET /api/v1/report-templates", s.require("data.export", s.listReportTemplates))
	mux.Handle("POST /api/v1/report-templates/preview", s.require("content.write", s.previewReportSource))
	mux.Handle("POST /api/v1/report-templates/compile", s.require("content.write", s.compileReportSpec))
	mux.Handle("GET /api/v1/report-templates/{kind}/default-spec", s.require("data.export", s.defaultReportSpec))
	mux.Handle("GET /api/v1/report-templates/{kind}", s.require("data.export", s.getReportTemplate))
	mux.Handle("GET /api/v1/report-templates/{kind}/contract", s.require("data.export", s.reportTemplateContract))
	mux.Handle("GET /api/v1/report-templates/{kind}/preview", s.require("data.export", s.previewReportTemplate))
	mux.Handle("PUT /api/v1/report-templates/{kind}", s.require("content.write", s.putReportTemplate))
	mux.Handle("DELETE /api/v1/report-templates/{kind}", s.require("content.write", s.deleteReportTemplate))
	mux.Handle("GET /api/v1/reports", s.require("data.export", s.listReports))
	mux.Handle("GET /api/v1/reports/{id}", s.require("data.export", s.getReport))
	mux.Handle("GET /api/v1/reports/{id}/file", s.require("data.export", s.getReportFile))

	// pages (the UI is a client of the same API; the footer with product name, GitHub and license is fixed)
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) { r.URL.Path = "/login.html"; files.ServeHTTP(w, r) })
	mux.HandleFunc("GET /forgot", func(w http.ResponseWriter, r *http.Request) { r.URL.Path = "/forgot.html"; files.ServeHTTP(w, r) })
	mux.HandleFunc("GET /reset", func(w http.ResponseWriter, r *http.Request) { r.URL.Path = "/reset.html"; files.ServeHTTP(w, r) })
	mux.HandleFunc("GET /register", func(w http.ResponseWriter, r *http.Request) { r.URL.Path = "/register.html"; files.ServeHTTP(w, r) })
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) { r.URL.Path = "/settings.html"; files.ServeHTTP(w, r) })
	mux.HandleFunc("GET /users", func(w http.ResponseWriter, r *http.Request) { r.URL.Path = "/users.html"; files.ServeHTTP(w, r) })
	mux.HandleFunc("GET /account", func(w http.ResponseWriter, r *http.Request) { r.URL.Path = "/account.html"; files.ServeHTTP(w, r) })
	mux.HandleFunc("GET /devices", func(w http.ResponseWriter, r *http.Request) { r.URL.Path = "/devices.html"; files.ServeHTTP(w, r) })
	mux.HandleFunc("GET /assets", func(w http.ResponseWriter, r *http.Request) { r.URL.Path = "/assets.html"; files.ServeHTTP(w, r) })
	mux.HandleFunc("GET /fota", func(w http.ResponseWriter, r *http.Request) { r.URL.Path = "/fota.html"; files.ServeHTTP(w, r) })
	// sub-pages: every section is its own screen; the page script picks the section from the last path segment
	page := func(file string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { r.URL.Path = "/" + file; files.ServeHTTP(w, r) }
	}
	mux.HandleFunc("GET /settings/{section}", page("settings.html"))
	mux.HandleFunc("GET /users/{section}", page("users.html"))
	mux.HandleFunc("GET /organizations", page("organizations.html"))
	mux.HandleFunc("GET /incidents", page("incidents.html"))
	mux.HandleFunc("GET /dashboards", page("dashboards.html"))
	mux.HandleFunc("GET /dashboards/{slug}", page("dashboards.html"))
	mux.HandleFunc("GET /dashboards/{slug}/{mode}", page("dashboards.html"))
	mux.HandleFunc("GET /p/{token}", page("public.html"))
	mux.HandleFunc("GET /display", page("display.html"))
	mux.HandleFunc("GET /displays", page("displays.html"))
	mux.HandleFunc("GET /api/docs", page("docs.html"))
	mux.HandleFunc("GET /home", page("home.html"))
	mux.HandleFunc("GET /energy", page("energy.html"))
	mux.HandleFunc("GET /landing/edit", page("landing-edit.html"))
	mux.HandleFunc("GET /video", page("video.html"))
	mux.HandleFunc("GET /video/{section}", page("video.html"))
	mux.HandleFunc("GET /video/cameras/{id}", page("camera.html"))
	mux.HandleFunc("GET /video/walls/{id}", page("wall.html"))
	mux.HandleFunc("GET /smarthome", page("smarthome.html"))
	mux.HandleFunc("GET /flows", page("flows.html"))
	mux.HandleFunc("GET /flows/{slug}", page("flows.html"))
	mux.HandleFunc("GET /server", page("server.html"))
	mux.HandleFunc("GET /server/{section}", page("server.html"))
	mux.HandleFunc("GET /fota/{section}", page("fota.html"))
	mux.HandleFunc("GET /account/{section}", page("account.html"))
	mux.HandleFunc("GET /assets/{section}", page("assets.html"))
	mux.HandleFunc("GET /devices/{id}", page("devices.html"))
	mux.HandleFunc("GET /devices/{id}/{tab}", page("devices.html"))
	mux.Handle("GET /", files)

	return countRequests(noStaleShell(s.secureHeaders(s.requestID(s.authenticate(mux)))))
}

// secureHeaders applies the baseline headers from ADR-0007 to every response.
func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https: http:; media-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		if s.isSecure(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	dbState := "ok"
	if err := s.pool.Ping(ctx); err != nil {
		dbState = "error"
	}
	clients := 0
	if s.br != nil {
		clients = s.br.Server.Clients.Len()
	}
	code := http.StatusOK
	if dbState != "ok" {
		code = http.StatusServiceUnavailable
	}
	ntp := "off"
	if ts := s.tc.State(); ts.Enabled {
		switch {
		case ts.OK:
			ntp = "ok"
		case ts.CheckedAt.IsZero():
			ntp = "pending"
		default:
			ntp = "warn"
		}
	}
	writeJSON(w, code, map[string]any{"status": dbState, "db": dbState, "mqtt_clients": clients, "version": s.version, "time": time.Now().UTC(), "ntp": ntp})
}

func (s *Server) datastreams(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `
		SELECT ds.id::text, ds.key, ds.quantity, ds.unit, ds.kind, a.name, a.id::text, COALESCE(ds.expected_interval::text, '')
		FROM core.datastream ds JOIN core.asset a ON a.id = ds.asset_id
		WHERE ($1 = '' OR ds.org_id = $1::uuid) ORDER BY a.name, ds.key`, orgOf(p))
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, key, quantity, unit, kind, asset, assetID, interval string
		if err := rows.Scan(&id, &key, &quantity, &unit, &kind, &asset, &assetID, &interval); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "key": key, "quantity": quantity, "unit": unit, "kind": kind, "asset": asset, "asset_id": assetID, "expected_interval": interval})
	}
	writeJSON(w, 200, out)
}

func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT id::text, external_id, name, transport, status, status->>'last_seen', revoked_at, location
		FROM core.device WHERE ($1 = '' OR org_id = $1::uuid) ORDER BY revoked_at NULLS FIRST, name`, orgOf(p))
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, ext, name, tr string
		var status json.RawMessage
		var lastSeen *string
		var revoked *time.Time
		var location json.RawMessage
		if err := rows.Scan(&id, &ext, &name, &tr, &status, &lastSeen, &revoked, &location); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "external_id": ext, "name": name, "transport": tr, "status": status, "last_seen": lastSeen, "revoked_at": revoked, "location": location, "connected": s.br != nil && s.br.IsConnected(id)})
	}
	writeJSON(w, 200, out)
}

func (s *Server) observations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dsID := q.Get("datastream_id")
	if dsID == "" {
		writeErr(w, 400, "datastream_id required")
		return
	}
	from := time.Now().Add(-24 * time.Hour)
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeErr(w, 400, "from must be RFC3339")
			return
		}
		from = t
	}
	to := time.Now().Add(time.Hour)
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeErr(w, 400, "to must be RFC3339")
			return
		}
		to = t
	}
	limit := 1000
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 10000 {
			limit = n
		}
	}
	if !to.After(from) {
		writeErr(w, 400, "to must be after from")
		return
	}
	if b := q.Get("bucket"); b != "" {
		n, err := strconv.Atoi(b)
		if err != nil {
			writeErr(w, 400, "bucket must be seconds")
			return
		}
		if n > 0 && to.Sub(from)/time.Duration(n)/time.Second > maxBuckets {
			writeErr(w, 400, "range too long for this bucket: at most 20000 buckets per request")
			return
		}
		s.bucketedObservations(w, r, dsID, from, to, n)
		return
	}
	if to.Sub(from) > maxRawRange {
		writeErr(w, 400, "range too long for raw rows: at most 366 days per request (use bucket or page by cursor)")
		return
	}
	// Cursor pagination (URS G-03): the cursor is the last row's (measured_at, id); rows are ordered by both so a
	// page boundary inside one timestamp is stable. X-Next-Cursor is set while more rows exist.
	cursorAt, cursorID, err := decodeCursor(q.Get("cursor"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if cursorAt != nil && cursorAt.After(from) {
		from = *cursorAt
	}
	p := PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `
		SELECT measured_at, received_at, value::float8, value_json, quality, time_source, id::text
		FROM core.observation WHERE datastream_id = $1::uuid AND ($5 = '' OR org_id = $5::uuid) AND measured_at >= $2 AND measured_at < $3
		AND ($6 = '' OR (measured_at, id) > ($2, $6::uuid))
		ORDER BY measured_at, id LIMIT $4`, dsID, from, to, limit+1, orgOf(p), cursorID)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	lastAt, lastID := time.Time{}, ""
	for rows.Next() {
		var mAt, rAt time.Time
		var val *float64
		var vj json.RawMessage
		var quality, ts, id string
		if err := rows.Scan(&mAt, &rAt, &val, &vj, &quality, &ts, &id); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		if len(out) == limit {
			// one row beyond the page: there is more
			next := encodeCursor(lastAt, lastID)
			w.Header().Set("X-Next-Cursor", next)
			nu := *r.URL
			nq := nu.Query()
			nq.Set("cursor", next)
			nu.RawQuery = nq.Encode()
			w.Header().Set("Link", "<"+nu.RequestURI()+">; rel=\"next\"")
			break
		}
		out = append(out, map[string]any{"measured_at": mAt, "received_at": rAt, "value": val, "value_json": vj, "quality": quality, "time_source": ts})
		lastAt, lastID = mAt, id
	}
	writeJSON(w, 200, out)
}

const (
	maxRawRange = 366 * 24 * time.Hour
	maxBuckets  = 20000
)

func encodeCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id))
}

// decodeCursor returns the (measured_at, id) a cursor points at; an empty cursor means the start.
func decodeCursor(c string) (*time.Time, string, error) {
	if c == "" {
		return nil, "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return nil, "", errors.New("invalid cursor")
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok || len(id) != 36 {
		return nil, "", errors.New("invalid cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, "", errors.New("invalid cursor")
	}
	return &t, id, nil
}

func (s *Server) alarms(w http.ResponseWriter, r *http.Request) {
	dsID := r.URL.Query().Get("datastream_id")
	p := PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `
		SELECT a.id::text, a.raised_at, a.cleared_at, a.state, a.severity, a.retrospective, a.value::float8, a.detail->>'key'
		FROM core.alarm a WHERE ($1 = '' OR a.datastream_id = NULLIF($1,'')::uuid) AND ($2 = '' OR a.org_id = $2::uuid) ORDER BY a.raised_at DESC LIMIT 200`, dsID, orgOf(p))
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, state, sev string
		var key *string
		var raised time.Time
		var cleared *time.Time
		var retro bool
		var val *float64
		if err := rows.Scan(&id, &raised, &cleared, &state, &sev, &retro, &val, &key); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "raised_at": raised, "cleared_at": cleared, "state": state, "severity": sev, "retrospective": retro, "value": val, "key": key})
	}
	writeJSON(w, 200, out)
}

func (s *Server) auditVerify(w http.ResponseWriter, r *http.Request) {
	res, err := audit.Verify(r.Context(), s.pool)
	if err != nil {
		writeErr(w, 500, "verify failed")
		return
	}
	if !res.OK {
		s.incidents.AuditChainBroken(r.Context(), seqOrZero(res.FirstBadSeq), res.Checked)
	}
	writeJSON(w, 200, map[string]any{"ok": res.OK, "checked": res.Checked, "first_bad_seq": res.FirstBadSeq, "head_seq": res.HeadSeq, "head_hash": res.HeadHash})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func seqOrZero(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
