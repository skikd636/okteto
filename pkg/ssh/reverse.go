// Copyright 2022 The Okteto Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	oktetoLog "github.com/okteto/okteto/pkg/log"
	"github.com/okteto/okteto/pkg/model"
)

type reverse struct {
	forward
}

// AddReverse adds a reverse forward
func (fm *ForwardManager) AddReverse(f model.Reverse) error {

	if err := fm.canAdd(f.Local, false); err != nil {
		return err
	}

	fm.reverses[f.Local] = &reverse{
		forward: forward{
			localAddress:  net.JoinHostPort(fm.localInterface, strconv.Itoa(f.Local)),
			remoteAddress: net.JoinHostPort(fm.remoteInterface, strconv.Itoa(f.Remote)),
		},
	}

	return nil
}

func (r *reverse) start(ctx context.Context) {
	remoteListener, err := r.pool.getListener(r.remoteAddress)
	if err != nil {
		oktetoLog.Infof("%s -> failed to listen on remote address: %v", r.String(), err)
		return
	}
	defer func() {
		if err := remoteListener.Close(); err != nil {
			oktetoLog.Debugf("Error closing remote listener '%s': %s", r.String(), err)
		}
	}()

	go func() {
		<-ctx.Done()
		r.setDisconnected()
		if err := remoteListener.Close(); err != nil {
			oktetoLog.Infof("%s -> failed to close: %s", r.String(), err)
		}

		oktetoLog.Infof("%s -> done", r.String())
	}()

	tick := time.NewTicker(500 * time.Millisecond)
	for {
		r.setConnected()
		remoteConn, err := remoteListener.Accept()
		if err != nil {
			if !r.connected() {
				return
			}

			oktetoLog.Infof("%s -> failed to accept connection: %v", r.String(), err)
			<-tick.C
			continue
		}

		go r.handle(ctx, remoteConn)

	}
}

func (r *reverse) handle(ctx context.Context, remote net.Conn) {
	defer func() {
		if err := remote.Close(); err != nil {
			oktetoLog.Debugf("Error closing remote connection: %s", err)
		}
	}()

	quit := make(chan struct{}, 1)
	local, err := getConn(ctx, r.localAddress, defaultRetries)
	if err != nil {
		oktetoLog.Infof("%s -> failed to listen on local address: %v", r.String(), err)
		return
	}

	defer func() {
		if err := local.Close(); err != nil {
			oktetoLog.Debugf("Error closing local connection: %s", err)
		}
	}()

	go r.transfer(remote, local, quit)
	go r.transfer(local, remote, quit)

	<-quit
}

func (r *reverse) String() string {
	return fmt.Sprintf("ssh reverse forward %s<-%s", r.localAddress, r.remoteAddress)
}

func (r *reverse) transfer(from io.Writer, to io.Reader, quit chan struct{}) {
	_, err := io.Copy(from, to)
	if err != nil {
		oktetoLog.Infof("%s -> data transfer failed: %v", r.String(), err)
	}

	quit <- struct{}{}
}
