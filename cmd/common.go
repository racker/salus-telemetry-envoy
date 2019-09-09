/*
 * Copyright 2019 Rackspace US, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"context"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func handleInterrupts(body func(ctx context.Context)) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill, syscall.SIGTERM)

	rootCtx, cancel := context.WithCancel(context.Background())

	body(rootCtx)

	for {
		select {
		case <-signalChan:
			logrus.Info("cancelling application context")
			cancel()
		case <-rootCtx.Done():
			// allow for agent processes to be notified
			time.Sleep(1 * time.Second)
			os.Exit(0)
		}
	}

}
