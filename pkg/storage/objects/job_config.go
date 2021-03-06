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

package objects

import (
	"bytes"
	"compress/gzip"
	"context"
	"io/ioutil"
	"time"

	"github.com/uber/peloton/.gen/peloton/api/v0/job"
	"github.com/uber/peloton/.gen/peloton/api/v0/peloton"
	"github.com/uber/peloton/.gen/peloton/private/models"

	"github.com/uber/peloton/pkg/storage/objects/base"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
)

// init adds a JobConfigObject instance to the global list of storage objects
func init() {
	Objs = append(Objs, &JobConfigObject{})
}

// JobConfigObject corresponds to a row in job_config table.
type JobConfigObject struct {
	// DB specific annotations
	base.Object `cassandra:"name=job_config, primaryKey=((job_id), version)"`

	// JobID of the job
	JobID string `column:"name=job_id"`
	// Number of task instances
	Version uint64 `column:"name=version"`
	// Config of the job
	Config []byte `column:"name=config"`
	// Config AddOn field for the job
	ConfigAddOn []byte `column:"name=config_addon"`
	// Creation time of the job
	CreationTime time.Time `column:"name=creation_time"`
}

// JobConfigOps provides methods for manipulating job_config table.
type JobConfigOps interface {
	// Create inserts a row in the table.
	Create(
		ctx context.Context,
		id *peloton.JobID,
		config *job.JobConfig,
		configAddOn *models.ConfigAddOn,
		version uint64,
	) error

	// Get retrieves a row from the table.
	Get(
		ctx context.Context,
		id *peloton.JobID,
		version uint64,
	) (*job.JobConfig, *models.ConfigAddOn, error)

	// Delete removes an object from the table.
	Delete(ctx context.Context, id *peloton.JobID, version uint64) error
}

// ensure that default implementation (jobConfigOps) satisfies the interface
var _ JobConfigOps = (*jobConfigOps)(nil)

// compress a blob using gzip
func compress(buffer []byte) ([]byte, error) {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(buffer); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Uncompress a blob using gzip, return original blob if it was not compressed
func uncompress(buffer []byte) ([]byte, error) {
	b := bytes.NewBuffer(buffer)
	r, err := gzip.NewReader(b)

	if err != nil {
		if err == gzip.ErrHeader {
			// blob was not compressed, so we can ignore this error. We can
			// look for only checksum errors which will mean data corruption
			return buffer, nil
		}
		return nil, err
	}

	defer r.Close()
	uncompressed, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return uncompressed, nil
}

func (j *JobConfigObject) toConfig() (*job.JobConfig, error) {
	configBuffer, err := uncompress(j.Config)
	if err != nil {
		return nil, err
	}
	config := &job.JobConfig{}
	err = proto.Unmarshal(configBuffer, config)
	return config, err
}

func (j *JobConfigObject) toConfigAddOn() (*models.ConfigAddOn, error) {
	addOn := &models.ConfigAddOn{}
	err := proto.Unmarshal(j.ConfigAddOn, addOn)
	return addOn, err
}

// newJobConfigObject creates a JobConfigObject from job config and runtime
func newJobConfigObject(
	id *peloton.JobID,
	version uint64,
	config *job.JobConfig,
	configAddOn *models.ConfigAddOn,
) (*JobConfigObject, error) {

	obj := &JobConfigObject{JobID: id.GetValue()}

	configBuffer, err := proto.Marshal(config)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to marshal jobConfig")
	}

	configBuffer, err = compress(configBuffer)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to compress jobConfig")
	}

	addOnBuffer, err := proto.Marshal(configAddOn)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to marshal configAddOn")
	}
	obj.Version = version
	obj.Config = configBuffer
	obj.ConfigAddOn = addOnBuffer
	obj.CreationTime = time.Now().UTC()
	return obj, nil
}

// jobConfigOps implements jobConfigOps using a particular Store
type jobConfigOps struct {
	store *Store
}

// NewJobConfigOps constructs a jobConfigOps object for provided Store.
func NewJobConfigOps(s *Store) JobConfigOps {
	return &jobConfigOps{store: s}
}

// Create creates a JobConfigObject in db
func (d *jobConfigOps) Create(
	ctx context.Context,
	id *peloton.JobID,
	config *job.JobConfig,
	configAddOn *models.ConfigAddOn,
	version uint64,
) error {

	obj, err := newJobConfigObject(id, version, config, configAddOn)
	if err != nil {
		d.store.metrics.OrmJobMetrics.JobConfigCreateFail.Inc(1)
		return errors.Wrap(err, "Failed to construct JobConfigObject")
	}

	if err = d.store.oClient.CreateIfNotExists(ctx, obj); err != nil {
		d.store.metrics.OrmJobMetrics.JobConfigCreateFail.Inc(1)
		return err
	}

	d.store.metrics.OrmJobMetrics.JobConfigCreate.Inc(1)
	return nil
}

// Get gets a JobConfigObject from db
func (d *jobConfigOps) Get(
	ctx context.Context,
	id *peloton.JobID,
	version uint64,
) (*job.JobConfig, *models.ConfigAddOn, error) {

	obj := &JobConfigObject{
		JobID:   id.GetValue(),
		Version: version,
	}

	if err := d.store.oClient.Get(ctx, obj); err != nil {
		d.store.metrics.OrmJobMetrics.JobConfigGetFail.Inc(1)
		return nil, nil, err
	}

	config, err := obj.toConfig()
	if err != nil {
		d.store.metrics.OrmJobMetrics.JobConfigGetFail.Inc(1)
		return nil, nil, errors.Wrap(err, "Failed to unmarshal config")
	}

	configAddOn, err := obj.toConfigAddOn()
	if err != nil {
		d.store.metrics.OrmJobMetrics.JobConfigGetFail.Inc(1)
		return nil, nil, errors.Wrap(err, "Failed to unmarshal configAddOn")
	}

	d.store.metrics.OrmJobMetrics.JobConfigGet.Inc(1)
	return config, configAddOn, nil
}

// Delete deletes a JobConfigObject from db
func (d *jobConfigOps) Delete(
	ctx context.Context,
	id *peloton.JobID,
	version uint64,
) error {
	obj := &JobConfigObject{
		JobID:   id.GetValue(),
		Version: version,
	}

	if err := d.store.oClient.Delete(ctx, obj); err != nil {
		d.store.metrics.OrmJobMetrics.JobConfigDeleteFail.Inc(1)
		return err
	}
	d.store.metrics.OrmJobMetrics.JobConfigDelete.Inc(1)
	return nil
}
