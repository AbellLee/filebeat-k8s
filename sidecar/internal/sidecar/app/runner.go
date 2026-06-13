package app

import (
	"context"
	"log/slog"
	"time"

	"filebeat-k8s/internal/control"
	"filebeat-k8s/sidecar/internal/sidecar/agent"
	"filebeat-k8s/sidecar/internal/sidecar/apply"
	"filebeat-k8s/sidecar/internal/sidecar/client"
	"filebeat-k8s/sidecar/internal/sidecar/config"
	"filebeat-k8s/sidecar/internal/sidecar/symlink"
)

type Runner struct {
	cfg     config.Config
	log     *slog.Logger
	client  *client.Client
	applier *apply.Applier
}

func New(cfg config.Config, logger *slog.Logger) *Runner {
	return &Runner{
		cfg:     cfg,
		log:     logger,
		client:  client.New(cfg.ControlServerURL, cfg.AgentToken, cfg.WatchTimeout),
		applier: apply.New(cfg.InputsDir),
	}
}

func (r *Runner) Run(ctx context.Context) error {
	identity, err := agent.Build(ctx, r.cfg.ClusterID, r.cfg.NodeName, r.cfg.PodName, r.cfg.PodNamespace, r.cfg.AgentVersion, r.cfg.FilebeatVersion, r.cfg.NodeLabels)
	if err != nil {
		return err
	}
	currentChecksum := r.applier.LoadLastChecksum()
	identity.CurrentConfigChecksum = currentChecksum

	if mgr, err := symlink.NewManager(symlink.Config{
		NodeName:            r.cfg.NodeName,
		KlogDir:             r.cfg.KlogDir,
		KlogStdioDir:        r.cfg.KlogStdioDir,
		HostFSDir:           r.cfg.HostFSDir,
		HostProcDir:         r.cfg.HostProcDir,
		ContainerdStateDir:  r.cfg.ContainerdStateDir,
		VarLogContainersDir: r.cfg.VarLogContainersDir,
		ReconcileInterval:   r.cfg.ReconcileInterval,
	}, r.log); err != nil {
		r.log.Warn("symlink-manager degraded", "error", err)
	} else {
		go mgr.Run(ctx)
	}

	if err := r.client.Register(ctx, control.AgentRegisterRequest{
		ID:                    identity.AgentID,
		ClusterID:             identity.ClusterID,
		NodeName:              identity.NodeName,
		PodName:               identity.PodName,
		Namespace:             identity.Namespace,
		AgentVersion:          identity.AgentVersion,
		FilebeatVersion:       identity.FilebeatVersion,
		CurrentConfigChecksum: identity.CurrentConfigChecksum,
		NodeLabels:            identity.NodeLabels,
	}); err != nil {
		return err
	}
	r.log.Info("registered agent", "agent_id", identity.AgentID)

	backoff := r.cfg.PollInterval
	for {
		if err := r.client.Heartbeat(ctx, control.AgentHeartbeatRequest{
			ID:                    identity.AgentID,
			ClusterID:             identity.ClusterID,
			NodeName:              identity.NodeName,
			CurrentConfigChecksum: currentChecksum,
		}); err != nil {
			r.log.Warn("heartbeat failed", "agent_id", identity.AgentID, "error", err)
		}

		resp, err := r.client.PullConfig(ctx, r.cfg.ConfigMode, identity.AgentID, identity.ClusterID, currentChecksum)
		if err != nil {
			r.log.Warn("pull config failed", "agent_id", identity.AgentID, "mode", r.cfg.ConfigMode, "error", err)
			if r.cfg.RunOnce {
				return err
			}
			sleep(ctx, backoff)
			if backoff < 5*r.cfg.PollInterval {
				backoff *= 2
			}
			continue
		}
		backoff = r.cfg.PollInterval

		if resp.Changed {
			if err := r.applier.Apply(resp); err != nil {
				r.log.Warn("apply config failed", "agent_id", identity.AgentID, "checksum", resp.Checksum, "error", err)
				_ = r.client.ReportApplyResult(ctx, control.AgentApplyResultRequest{AgentID: identity.AgentID, Checksum: resp.Checksum, Status: "failed", Message: err.Error()})
			} else {
				currentChecksum = resp.Checksum
				r.log.Info("applied config", "agent_id", identity.AgentID, "checksum", currentChecksum, "files", len(resp.Files))
				_ = r.client.ReportApplyResult(ctx, control.AgentApplyResultRequest{AgentID: identity.AgentID, Checksum: resp.Checksum, Status: "success", Message: "applied"})
				if err := r.client.Heartbeat(ctx, control.AgentHeartbeatRequest{
					ID:                    identity.AgentID,
					ClusterID:             identity.ClusterID,
					NodeName:              identity.NodeName,
					CurrentConfigChecksum: currentChecksum,
				}); err != nil {
					r.log.Warn("post-apply heartbeat failed", "agent_id", identity.AgentID, "error", err)
				}
			}
		}

		if r.cfg.RunOnce {
			return nil
		}
		sleep(ctx, r.loopSleep())
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (r *Runner) loopSleep() time.Duration {
	if r.cfg.ConfigMode == "watch" {
		return time.Second
	}
	return r.cfg.PollInterval
}

func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
