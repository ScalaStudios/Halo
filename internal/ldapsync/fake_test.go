package ldapsync

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"
	"github.com/go-ldap/ldap/v3"
)

type fakeEntry struct {
	dn    string
	attrs map[string][]string
}

type fakeLDAP struct {
	mu       sync.Mutex
	url      string
	ca       string
	bindDN   string
	password string
	startTLS bool
	cert     tls.Certificate
	entries  []fakeEntry
	binds    int
	pages    []uint32
}

func certificate(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func serveLDAP(t *testing.T, mode string) *fakeLDAP {
	t.Helper()
	f := &fakeLDAP{bindDN: "cn=halo,dc=example,dc=com", password: "s3cret", startTLS: mode == "starttls"}
	f.cert, f.ca = certificate(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.url = "ldap://" + ln.Addr().String()
	if mode == "ldaps" {
		ln = tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{f.cert}})
		f.url = "ldaps://" + ln.Addr().String()
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	return f
}

func (f *fakeLDAP) set(entries ...fakeEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = entries
}

func (f *fakeLDAP) stats() (int, []uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.binds, slices.Clone(f.pages)
}

func response(messageID int64, tag ber.Tag, code int64, controls ...ldap.Control) *ber.Packet {
	p := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "")
	p.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger, messageID, ""))
	op := ber.Encode(ber.ClassApplication, ber.TypeConstructed, tag, nil, "")
	op.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagEnumerated, code, ""))
	op.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", ""))
	op.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", ""))
	p.AppendChild(op)
	if len(controls) > 0 {
		list := ber.Encode(ber.ClassContext, ber.TypeConstructed, 0, nil, "")
		for _, c := range controls {
			list.AppendChild(c.Encode())
		}
		p.AppendChild(list)
	}
	return p
}

func entryPacket(messageID int64, e fakeEntry) *ber.Packet {
	p := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "")
	p.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger, messageID, ""))
	op := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ldap.ApplicationSearchResultEntry, nil, "")
	op.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, e.dn, ""))
	attrs := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "")
	for name, values := range e.attrs {
		attr := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "")
		attr.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, name, ""))
		set := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSet, nil, "")
		for _, v := range values {
			set.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, v, ""))
		}
		attr.AppendChild(set)
		attrs.AppendChild(attr)
	}
	op.AppendChild(attrs)
	p.AppendChild(op)
	return p
}

func (f *fakeLDAP) serve(conn net.Conn) {
	defer func() { conn.Close() }()
	for {
		packet, err := ber.ReadPacket(conn)
		if err != nil || len(packet.Children) < 2 {
			return
		}
		messageID, _ := packet.Children[0].Value.(int64)
		op := packet.Children[1]
		var out []*ber.Packet
		switch op.Tag {
		case ldap.ApplicationBindRequest:
			f.mu.Lock()
			f.binds++
			ok := op.Children[1].Value == f.bindDN && op.Children[2].Data.String() == f.password
			f.mu.Unlock()
			out = append(out, response(messageID, ldap.ApplicationBindResponse, map[bool]int64{true: ldap.LDAPResultSuccess, false: ldap.LDAPResultInvalidCredentials}[ok]))
		case ldap.ApplicationExtendedRequest:
			if !f.startTLS {
				conn.Write(response(messageID, ldap.ApplicationExtendedResponse, ldap.LDAPResultProtocolError).Bytes())
				continue
			}
			conn.Write(response(messageID, ldap.ApplicationExtendedResponse, ldap.LDAPResultSuccess).Bytes())
			secure := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{f.cert}})
			if secure.Handshake() != nil {
				return
			}
			conn = secure
			continue
		case ldap.ApplicationSearchRequest:
			out = f.search(messageID, packet)
		case ldap.ApplicationUnbindRequest:
			return
		}
		for _, p := range out {
			if _, err := conn.Write(p.Bytes()); err != nil {
				return
			}
		}
	}
}

func (f *fakeLDAP) search(messageID int64, packet *ber.Packet) []*ber.Packet {
	base, _ := packet.Children[1].Children[0].Value.(string)
	var paging *ldap.ControlPaging
	if len(packet.Children) > 2 {
		for _, child := range packet.Children[2].Children {
			if c, err := ldap.DecodeControl(child); err == nil {
				if p, ok := c.(*ldap.ControlPaging); ok {
					paging = p
				}
			}
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var matched []fakeEntry
	for _, e := range f.entries {
		if strings.HasSuffix(strings.ToLower(e.dn), ","+strings.ToLower(base)) {
			matched = append(matched, e)
		}
	}
	if paging == nil {
		var out []*ber.Packet
		for _, e := range matched {
			out = append(out, entryPacket(messageID, e))
		}
		return append(out, response(messageID, ldap.ApplicationSearchResultDone, ldap.LDAPResultSuccess))
	}
	f.pages = append(f.pages, paging.PagingSize)
	start, _ := strconv.Atoi(string(paging.Cookie))
	end := min(start+int(paging.PagingSize), len(matched))
	if paging.PagingSize == 0 {
		end = start
	}
	var out []*ber.Packet
	for _, e := range matched[start:end] {
		out = append(out, entryPacket(messageID, e))
	}
	next := ldap.NewControlPaging(0)
	if end < len(matched) && paging.PagingSize > 0 {
		next.SetCookie([]byte(strconv.Itoa(end)))
	}
	return append(out, response(messageID, ldap.ApplicationSearchResultDone, ldap.LDAPResultSuccess, next))
}
