// Copyright 2021 The Okteto Authors
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

package build

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/okteto/okteto/pkg/analytics"
	okErrors "github.com/okteto/okteto/pkg/errors"
	"github.com/okteto/okteto/pkg/log"
	"github.com/okteto/okteto/pkg/okteto"
	"github.com/okteto/okteto/pkg/registry"
	"github.com/pkg/errors"
)

// Run runs the build sequence
func Run(ctx context.Context, path, dockerFile, tag, target string, noCache bool, cacheFrom, buildArgs, secrets []string, progress string) error {
	buildkitClient, err := getBuildkitClient(ctx)
	if err != nil {
		return err
	}

	if dockerFile != "" {
		dockerFile, err = registry.GetDockerfile(dockerFile)
		if err != nil {
			return err
		}
		defer os.Remove(dockerFile)
	}

	if tag != "" {
		err = validateImage(tag)
		if err != nil {
			return err
		}
	}
	if okteto.IsOktetoContext() {
		tag = registry.ExpandOktetoDevRegistry(tag)
		tag = registry.ExpandOktetoGlobalRegistry(tag)
		for i := range cacheFrom {
			cacheFrom[i] = registry.ExpandOktetoDevRegistry(cacheFrom[i])
			cacheFrom[i] = registry.ExpandOktetoGlobalRegistry(cacheFrom[i])
		}
	}
	opt, err := getSolveOpt(path, dockerFile, tag, target, noCache, cacheFrom, buildArgs, secrets)
	if err != nil {
		return errors.Wrap(err, "failed to create build solver")
	}

	err = solveBuild(ctx, buildkitClient, opt, progress)
	if err != nil {
		log.Infof("Failed to build image: %s", err.Error())
	}
	if registry.IsTransientError(err) {
		log.Yellow(`Failed to push '%s' to the registry:
  %s,
  Retrying ...`, tag, err.Error())
		success := true
		err := solveBuild(ctx, buildkitClient, opt, progress)
		if err != nil {
			success = false
			log.Infof("Failed to build image: %s", err.Error())
		}
		err = registry.GetErrorMessage(err, tag)
		analytics.TrackBuildTransientError(okteto.Context().Buildkit, success)
		return err
	}

	err = registry.GetErrorMessage(err, tag)

	return err
}

func validateImage(imageTag string) error {
	if (registry.IsOktetoRegistry(imageTag)) && strings.Count(imageTag, "/") != 1 {
		prefix := okteto.DevRegistry
		if registry.IsGlobalRegistry(imageTag) {
			prefix = okteto.GlobalRegistry
		}
		return okErrors.UserError{
			E:    fmt.Errorf("Can not use '%s' as the image tag.", imageTag),
			Hint: fmt.Sprintf("The syntax for using okteto registry is: '%s/image_name'", prefix),
		}
	}
	return nil
}
