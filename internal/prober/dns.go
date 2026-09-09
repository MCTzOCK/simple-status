package prober

import (
	"context"
	"fmt"
	"net"
	"time"

	"simple-status/internal/config"
)

func init() { Register("dns", newDNSProbe) }

// dnsResolverTimeout is applied on top of the check deadline because DNS
// lookups bypass the context in some edge cases of the net package.
const dnsResolverTimeout = 10 * time.Second

// dnsProbe resolves a hostname and treats a non-empty answer as success.
type dnsProbe struct {
	host   string
	record string
	res    *net.Resolver
}

func newDNSProbe(svc config.Service) (Probe, error) {
	switch svc.RecordType {
	case "A", "AAAA", "CNAME", "MX", "NS", "TXT":
	default:
		return nil, fmt.Errorf("dns record type %q must be one of A, AAAA, CNAME, MX, NS, TXT", svc.RecordType)
	}

	p := &dnsProbe{host: svc.Target, record: svc.RecordType, res: net.DefaultResolver}
	if svc.Resolver != "" {
		resolver := svc.Resolver
		if _, _, err := net.SplitHostPort(resolver); err != nil {
			resolver = net.JoinHostPort(resolver, "53")
		}
		p.res = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, resolver)
			},
		}
	}
	return p, nil
}

func (p *dnsProbe) Check(ctx context.Context) Result {
	ctx, cancel := context.WithTimeout(ctx, dnsResolverTimeout)
	defer cancel()

	var count int
	var err error
	switch p.record {
	case "A", "AAAA":
		count, err = p.lookupIP(ctx)
	case "CNAME":
		var cname string
		cname, err = p.res.LookupCNAME(ctx, p.host)
		if err == nil && cname == "" {
			err = fmt.Errorf("no CNAME record found")
		} else if err == nil {
			count = 1
		}
	case "MX":
		var mx []*net.MX
		mx, err = p.res.LookupMX(ctx, p.host)
		count = len(mx)
	case "NS":
		var ns []*net.NS
		ns, err = p.res.LookupNS(ctx, p.host)
		count = len(ns)
	case "TXT":
		var txt []string
		txt, err = p.res.LookupTXT(ctx, p.host)
		count = len(txt)
	default:
		err = fmt.Errorf("unsupported record type %q", p.record)
	}

	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	if count == 0 {
		return Result{Success: false, Message: fmt.Sprintf("no %s records found", p.record)}
	}
	plural := "s"
	if count == 1 {
		plural = ""
	}
	return Result{Success: true, Message: fmt.Sprintf("%d %s record%s found", count, p.record, plural)}
}

// lookupIP counts the addresses matching the probe's address family.
func (p *dnsProbe) lookupIP(ctx context.Context) (int, error) {
	addrs, err := p.res.LookupIPAddr(ctx, p.host)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, addr := range addrs {
		if (p.record == "A" && addr.IP.To4() != nil) ||
			(p.record == "AAAA" && addr.IP.To4() == nil) {
			count++
		}
	}
	return count, nil
}
