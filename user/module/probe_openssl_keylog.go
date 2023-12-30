// Copyright 2022 CFC4N <cfc4n.cs@gmail.com>. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package module

import (
	"ecapture/user/config"
	"ecapture/user/event"
	"errors"
	"github.com/cilium/ebpf"
	manager "github.com/gojue/ebpfmanager"
	"golang.org/x/sys/unix"
	"math"
	"strings"
)

func (m *MOpenSSLProbe) setupManagersKeylog() error {
	var binaryPath, sslVersion string

	sslVersion = m.conf.(*config.OpensslConfig).SslVersion
	sslVersion = strings.ToLower(sslVersion)
	switch m.conf.(*config.OpensslConfig).ElfType {
	//case config.ElfTypeBin:
	//	binaryPath = m.conf.(*config.OpensslConfig).Curlpath
	case config.ElfTypeSo:
		binaryPath = m.conf.(*config.OpensslConfig).Openssl
		err := m.getSslBpfFile(binaryPath, sslVersion)
		if err != nil {
			return err
		}
	default:
		//如果没找到
		binaryPath = "/lib/x86_64-linux-gnu/libssl.so.1.1"
		err := m.getSslBpfFile(binaryPath, sslVersion)
		if err != nil {
			return err
		}
	}

	m.logger.Printf("%s\tHOOK type:%d, binrayPath:%s\n", m.Name(), m.conf.(*config.OpensslConfig).ElfType, binaryPath)
	m.logger.Printf("%s\tHook masterKey function:%s\n", m.Name(), m.masterHookFunc)

	m.bpfManager = &manager.Manager{
		Probes: []*manager.Probe{
			// openssl masterkey
			{
				Section:          "uprobe/SSL_write_key",
				EbpfFuncName:     "probe_ssl_master_key",
				AttachToFuncName: m.masterHookFunc, // SSL_do_handshake or SSL_write
				BinaryPath:       binaryPath,
				UID:              "uprobe_ssl_master_key",
			},
		},

		Maps: []*manager.Map{
			{
				Name: "mastersecret_events",
			},
		},
	}

	m.bpfManagerOptions = manager.Options{
		DefaultKProbeMaxActive: 512,

		VerifierOptions: ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{
				LogSize: 2097152,
			},
		},

		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
	}

	if m.conf.EnableGlobalVar() {
		// 填充 RewriteContants 对应map
		m.bpfManagerOptions.ConstantEditors = m.constantEditor()
	}
	return nil
}

func (m *MOpenSSLProbe) initDecodeFunKeylog() error {
	MasterkeyEventsMap, found, err := m.bpfManager.GetMap("mastersecret_events")
	if err != nil {
		return err
	}
	if !found {
		return errors.New("cant found map:mastersecret_events")
	}
	m.eventMaps = append(m.eventMaps, MasterkeyEventsMap)

	var masterkeyEvent event.IEventStruct

	if m.isBoringSSL {
		masterkeyEvent = &event.MasterSecretBSSLEvent{}
	} else {
		masterkeyEvent = &event.MasterSecretEvent{}
	}

	//masterkeyEvent.SetModule(m)
	m.eventFuncMaps[MasterkeyEventsMap] = masterkeyEvent
	return nil
}