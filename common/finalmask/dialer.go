package finalmask

import (
	"context"
	"net"

	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// NewDialer оборачивает TCP-соединения dialer'а tcp-масками. Первая маска
// списка ближе всего к приложению, последняя — к сокету, как у Xray.
//
// ponytail: обёртка видна только как N.Dialer, поэтому параллельные
// network_strategy/интерфейсы с маской сводятся к обычному DialContext;
// пробросить ParallelInterfaceDialer, если такое сочетание понадобится.
func NewDialer(dialer N.Dialer, options *option.FinalMaskOptions) (N.Dialer, error) {
	if options == nil || len(options.TCP) == 0 {
		return dialer, nil
	}
	configs := make([]*fragmentConfig, 0, len(options.TCP))
	for _, mask := range options.TCP {
		config, err := newFragmentConfig(mask.Fragment)
		if err != nil {
			return nil, E.Cause(err, "finalmask")
		}
		configs = append(configs, config)
	}
	return &maskDialer{Dialer: dialer, tcp: configs}, nil
}

type maskDialer struct {
	N.Dialer
	tcp []*fragmentConfig
}

func (d *maskDialer) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	conn, err := d.Dialer.DialContext(ctx, network, destination)
	if err != nil || N.NetworkName(network) != N.NetworkTCP {
		return conn, err
	}
	for i := len(d.tcp) - 1; i >= 0; i-- {
		conn = &fragmentConn{Conn: conn, config: d.tcp[i]}
	}
	return conn, nil
}
