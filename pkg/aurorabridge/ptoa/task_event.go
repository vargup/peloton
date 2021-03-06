// Copyright (c) 2019 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ptoa

import (
	"time"

	"github.com/uber/peloton/.gen/peloton/api/v1alpha/pod"
	"github.com/uber/peloton/.gen/thrift/aurora/api"

	"go.uber.org/thriftrw/ptr"
)

const _auroraSchedulerName = "peloton"

// NewTaskEvent converts Peloton PodEvent to Aurora TaskEvent.
func NewTaskEvent(e *pod.PodEvent) (*api.TaskEvent, error) {
	t, err := time.Parse(time.RFC3339, e.GetTimestamp())
	if err != nil {
		return nil, err
	}
	s, err := convertTaskStateStringToScheduleStatus(e.GetActualState())
	if err != nil {
		return nil, err
	}

	return &api.TaskEvent{
		Timestamp: ptr.Int64(t.Unix() * 1000),
		Message:   ptr.String(e.GetMessage()),
		Status:    s,
		// TODO(kevinxu): does it need to be "Aurora"?
		Scheduler: ptr.String(_auroraSchedulerName),
	}, nil
}
