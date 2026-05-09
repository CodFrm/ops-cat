package assettype

import (
	"context"
	"fmt"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
)

type serialHandler struct{}

func init() {
	Register(&serialHandler{})
	policy.RegisterDefaultPolicy("serial", func() any { return asset_entity.DefaultCommandPolicy() })
}

func (h *serialHandler) Type() string     { return asset_entity.AssetTypeSerial }
func (h *serialHandler) DefaultPort() int { return 0 }

func (h *serialHandler) SafeView(a *asset_entity.Asset) map[string]any {
	cfg, err := a.GetSerialConfig()
	if err != nil || cfg == nil {
		return nil
	}
	return map[string]any{
		"port_path":    cfg.PortPath,
		"baud_rate":    cfg.BaudRate,
		"data_bits":    cfg.DataBits,
		"stop_bits":    cfg.StopBits,
		"parity":       cfg.Parity,
		"flow_control": cfg.FlowControl,
	}
}

// ResolvePassword 串口无密码，返回空。
func (h *serialHandler) ResolvePassword(_ context.Context, _ *asset_entity.Asset) (string, error) {
	return "", nil
}

func (h *serialHandler) DefaultPolicy() any { return asset_entity.DefaultCommandPolicy() }

func (h *serialHandler) ValidateCreateArgs(args map[string]any) error {
	if ArgString(args, "port_path") == "" {
		return fmt.Errorf("missing required parameter: port_path for serial type")
	}
	if ArgInt(args, "baud_rate") == 0 {
		return fmt.Errorf("missing required parameter: baud_rate for serial type")
	}
	return nil
}

func (h *serialHandler) ApplyCreateArgs(_ context.Context, a *asset_entity.Asset, args map[string]any) error {
	cfg := &asset_entity.SerialConfig{
		PortPath:    ArgString(args, "port_path"),
		BaudRate:    ArgInt(args, "baud_rate"),
		DataBits:    ArgInt(args, "data_bits"),
		StopBits:    ArgString(args, "stop_bits"),
		Parity:      ArgString(args, "parity"),
		FlowControl: ArgString(args, "flow_control"),
	}
	if cfg.DataBits == 0 {
		cfg.DataBits = 8
	}
	if cfg.StopBits == "" {
		cfg.StopBits = "1"
	}
	if cfg.Parity == "" {
		cfg.Parity = "none"
	}
	return a.SetSerialConfig(cfg)
}

func (h *serialHandler) ApplyUpdateArgs(_ context.Context, a *asset_entity.Asset, args map[string]any) error {
	cfg, err := a.GetSerialConfig()
	if err != nil || cfg == nil {
		return err
	}
	if v := ArgString(args, "port_path"); v != "" {
		cfg.PortPath = v
	}
	if v := ArgInt(args, "baud_rate"); v != 0 {
		cfg.BaudRate = v
	}
	if v := ArgInt(args, "data_bits"); v != 0 {
		cfg.DataBits = v
	}
	if v := ArgString(args, "stop_bits"); v != "" {
		cfg.StopBits = v
	}
	if v := ArgString(args, "parity"); v != "" {
		cfg.Parity = v
	}
	if v := ArgString(args, "flow_control"); v != "" {
		cfg.FlowControl = v
	}
	return a.SetSerialConfig(cfg)
}
