package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
)

type ACMAPI interface {
	ListCertificates(ctx context.Context, in *acm.ListCertificatesInput, opts ...func(*acm.Options)) (*acm.ListCertificatesOutput, error)
	DescribeCertificate(ctx context.Context, in *acm.DescribeCertificateInput, opts ...func(*acm.Options)) (*acm.DescribeCertificateOutput, error)
}

type ACMProvider struct{ client ACMAPI }

func NewACMProvider(cfg awssdk.Config, endpointURL string) *ACMProvider {
	var opts []func(*acm.Options)
	if endpointURL != "" {
		opts = append(opts, func(o *acm.Options) { o.BaseEndpoint = awssdk.String(endpointURL) })
	}
	return &ACMProvider{client: acm.NewFromConfig(cfg, opts...)}
}

func NewACMProviderWithClient(client ACMAPI) *ACMProvider { return &ACMProvider{client: client} }

func (p *ACMProvider) Name() string { return "ACM" }

func (p *ACMProvider) ListItems(ctx context.Context, query string) ([]Item, error) {
	out, err := p.client.ListCertificates(ctx, &acm.ListCertificatesInput{})
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}
	items := make([]Item, len(out.CertificateSummaryList))
	for i, c := range out.CertificateSummaryList {
		items[i] = Item{
			ID:   awssdk.ToString(c.CertificateArn),
			Name: awssdk.ToString(c.DomainName),
		}
	}
	return filterItems(items, query), nil
}

func (p *ACMProvider) GetDetail(ctx context.Context, item Item) (string, error) {
	return p.tabOverview(ctx, item)
}

func (p *ACMProvider) Tabs() []TabDef {
	return []TabDef{
		{Label: "Overview", Fetch: p.tabOverview},
		{Label: "Domains", Fetch: p.tabDomains},
		{Label: "Validation", Fetch: p.tabValidation},
	}
}

func (p *ACMProvider) tabOverview(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeCertificate(ctx, &acm.DescribeCertificateInput{CertificateArn: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	cert := out.Certificate

	expires := "(not set)"
	if cert.NotAfter != nil {
		days := int(time.Until(*cert.NotAfter).Hours() / 24)
		expires = fmt.Sprintf("%s  (%d days)", cert.NotAfter.Format(time.DateOnly), days)
	}

	inUseBy := "(none)"
	if len(cert.InUseBy) > 0 {
		inUseBy = cert.InUseBy[0]
	}

	return KV([][2]string{
		{"Domain", awssdk.ToString(cert.DomainName)},
		{"Status", string(cert.Status)},
		{"Type", string(cert.Type)},
		{"Expires", expires},
		{"Key", string(cert.KeyAlgorithm)},
		{"In Use By", inUseBy},
	}), nil
}

func (p *ACMProvider) tabDomains(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeCertificate(ctx, &acm.DescribeCertificateInput{CertificateArn: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	cert := out.Certificate
	if len(cert.SubjectAlternativeNames) == 0 {
		return "  (no additional domains)\n", nil
	}
	primary := awssdk.ToString(cert.DomainName)
	var sb strings.Builder
	for _, name := range cert.SubjectAlternativeNames {
		if name == primary {
			fmt.Fprintf(&sb, "  %s  (primary)\n", name)
		} else {
			fmt.Fprintf(&sb, "  %s\n", name)
		}
	}
	return sb.String(), nil
}

func (p *ACMProvider) tabValidation(ctx context.Context, item Item) (string, error) {
	out, err := p.client.DescribeCertificate(ctx, &acm.DescribeCertificateInput{CertificateArn: awssdk.String(item.ID)})
	if err != nil {
		return "", err
	}
	cert := out.Certificate
	if len(cert.DomainValidationOptions) == 0 {
		return "  (no validation options)\n", nil
	}

	method := string(cert.DomainValidationOptions[0].ValidationMethod)
	result := fmt.Sprintf("  Method:  %s\n\n", method)

	if cert.DomainValidationOptions[0].ValidationMethod == acmtypes.ValidationMethodDns {
		var rows [][]string
		for _, opt := range cert.DomainValidationOptions {
			if opt.ResourceRecord == nil {
				continue
			}
			rows = append(rows, []string{
				awssdk.ToString(opt.DomainName),
				truncate(awssdk.ToString(opt.ResourceRecord.Name), 30),
				truncate(awssdk.ToString(opt.ResourceRecord.Value), 30),
			})
		}
		if len(rows) > 0 {
			result += Table([]string{"Domain", "Record Name", "Record Value"}, rows)
		}
	}
	return result, nil
}
