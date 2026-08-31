package nodeinit

import (
	"github.com/opscores/umesh-cli/internal/yamlconfig"
)

func ToGenesisParams(cfg *yamlconfig.YamlNodeConfig, keyringPass string) GenesisParams {
	var vp yamlconfig.ValidatorInfo
	if cfg.Validator != nil {
		vp = *cfg.Validator
	}
	return GenesisParams{
		ChainID:                    cfg.Chain.ChainID,
		Moniker:                    cfg.Node.Moniker,
		Denom:                      cfg.Chain.Denom,
		MinGasPrice:                cfg.Chain.MinGasPrice,
		Environment:                cfg.Node.Environment,
		KeyringPass:                keyringPass,
		ValidatorName:              vp.KeyName,
		StakeAmount:                vp.StakeAmount,
		SelfDelegation:             vp.SelfDelegation,
		ExternalIP:                 vp.ExternalAddress,
		CommissionRate:             vp.Commission.Rate,
		CommissionMaxRate:          vp.Commission.MaxRate,
		CommissionMaxChange:        vp.Commission.MaxChangeRate,
		ValidatorMinSelfDelegation: vp.MinSelfDelegation,
		DenomURI:                   cfg.Chain.DenomURI,
	}
}

func ToValidatorParams(cfg *yamlconfig.YamlNodeConfig, keyringPass string) ValidatorParams {
	var join yamlconfig.JoinInfo
	if cfg.Join != nil {
		join = *cfg.Join
	}
	var net yamlconfig.NetworkInfo
	if cfg.Network != nil {
		net = *cfg.Network
	}

	return ValidatorParams{
		ChainID:         cfg.Chain.ChainID,
		Moniker:         cfg.Node.Moniker,
		Denom:           cfg.Chain.Denom,
		MinGasPrice:     cfg.Chain.MinGasPrice,
		Environment:     cfg.Node.Environment,
		KeyringPass:     keyringPass,
		SentryRPC:       join.SentryRPC,
		ValidatorRPC:    join.ValidatorRPC,
		GenesisURL:      join.GenesisURL,
		GenesisSHA256:   join.GenesisSHA256,
		Seeds:           net.Seeds,
		PersistentPeers: net.PersistentPeers,
		ExternalIP:      net.ExternalAddress,
	}
}

func ToSentryParams(cfg *yamlconfig.YamlNodeConfig) SentryParams {
	var join yamlconfig.JoinInfo
	if cfg.Join != nil {
		join = *cfg.Join
	}
	var net yamlconfig.NetworkInfo
	if cfg.Network != nil {
		net = *cfg.Network
	}
	return SentryParams{
		ChainID:         cfg.Chain.ChainID,
		Moniker:         cfg.Node.Moniker,
		Denom:           cfg.Chain.Denom,
		MinGasPrice:     cfg.Chain.MinGasPrice,
		Environment:     cfg.Node.Environment,
		SentryRPC:       join.SentryRPC,
		ValidatorRPC:    join.ValidatorRPC,
		GenesisURL:      join.GenesisURL,
		GenesisSHA256:   join.GenesisSHA256,
		Seeds:           net.Seeds,
		PersistentPeers: net.PersistentPeers,
		ExternalIP:      net.ExternalAddress,
		PublicIP:        net.PublicIP,
		ExternalPort:    net.ExternalPort,
		UsePrivate:      net.UsePrivate,
	}
}

func ToRPCParams(cfg *yamlconfig.YamlNodeConfig) RPCParams {
	var join yamlconfig.JoinInfo
	if cfg.Join != nil {
		join = *cfg.Join
	}
	var net yamlconfig.NetworkInfo
	if cfg.Network != nil {
		net = *cfg.Network
	}
	return RPCParams{
		ChainID:         cfg.Chain.ChainID,
		Moniker:         cfg.Node.Moniker,
		Denom:           cfg.Chain.Denom,
		MinGasPrice:     cfg.Chain.MinGasPrice,
		Environment:     cfg.Node.Environment,
		Pruning:         cfg.Node.Pruning,
		RPCUpstream:     join.SentryRPC,
		SentryRPC:       join.SentryRPC,
		GenesisURL:      join.GenesisURL,
		GenesisSHA256:   join.GenesisSHA256,
		ValidatorRPC:    join.ValidatorRPC,
		Seeds:           net.Seeds,
		PersistentPeers: net.PersistentPeers,
		ExternalIP:      net.ExternalAddress,
	}
}
