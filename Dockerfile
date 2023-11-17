# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

################################################################################
##                               BUILD ARGS                                   ##
################################################################################
# This build arg allows the specification of a custom Golang image.
ARG GOLANG_IMAGE=golang:1.20.7

# The distroless image on which the CPI manager image is built.
#
# Please do not use "latest". Explicit tags should be used to provide
# deterministic builds. Follow what kubernetes uses to build
# kube-controller-manager, for example for 1.27.x:
# https://github.com/kubernetes/kubernetes/blob/release-1.27/build/common.sh#L99
ARG DISTROLESS_IMAGE=registry.k8s.io/build-image/go-runner:v2.3.1-go1.20.7-bullseye.0

# We use Alpine as the source for default CA certificates and some output
# images
ARG ALPINE_IMAGE=alpine:3.17.5

# vcontainer-storage-interface uses Debian as a base image
ARG DEBIAN_IMAGE=registry.k8s.io/build-image/debian-base:bullseye-v1.4.3

################################################################################
##                              BUILD STAGE                                   ##
################################################################################

# Build an image containing a common ca-certificates used by all target images
# regardless of how they are built. We arbitrarily take ca-certificates from
# the amd64 Alpine image.
FROM --platform=linux/amd64 ${ALPINE_IMAGE} as certs
RUN apk add --no-cache ca-certificates


# Build all command targets. We build all command targets in a single build
# stage for efficiency. Target images copy their binary from this image.
# We use go's native cross compilation for multi-arch in this stage, so the
# builder itself is always amd64
FROM --platform=linux/amd64 ${GOLANG_IMAGE} as builder

ARG GOPROXY=https://goproxy.io,direct
ARG TARGETOS
ARG TARGETARCH
ARG VERSION

WORKDIR /build
COPY Makefile go.mod go.sum vcontainer-storage-interface ./
COPY cmd/ cmd/
RUN chmod +x vcontainer-storage-interface
#COPY pkg/ pkg/
# RUN make build GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOPROXY=${GOPROXY} VERSION=${VERSION}

################################################################################
##                             TARGET IMAGES                                  ##
################################################################################
##
## vcontainer-storage-interface
##

# step 1: copy all necessary files from Debian distro to /dest folder
# all magic heppens in tools/csi-deps.sh
FROM --platform=${TARGETPLATFORM} ${DEBIAN_IMAGE} as vcontainer-csi-plugin-utils

RUN clean-install bash rsync mount udev btrfs-progs e2fsprogs xfsprogs
COPY tools/csi-deps.sh /tools/csi-deps.sh
RUN chmod +x /tools/csi-deps.sh && /tools/csi-deps.sh

# step 2: check if all necessary files are copied and work properly
# the build have to finish without errors, but the result image will not be used
FROM --platform=${TARGETPLATFORM} ${DISTROLESS_IMAGE} as vcontainer-csi-plugin-utils-check

COPY --from=vcontainer-csi-plugin-utils /dest /
COPY --from=vcontainer-csi-plugin-utils /bin/sh /bin/sh
COPY tools/csi-deps-check.sh /tools/csi-deps-check.sh

SHELL ["/bin/sh"]
RUN /tools/csi-deps-check.sh

# step 3: build tiny vcontainer-storage-interface image with only necessary files
FROM --platform=${TARGETPLATFORM} ${DISTROLESS_IMAGE} as vcontainer-storage-interface

# Copying csi-deps-check.sh simply ensures that the resulting image has a dependency
# on vcontainer-csi-plugin-utils-check and therefore that the check has passed
COPY --from=vcontainer-csi-plugin-utils-check /tools/csi-deps-check.sh /bin/csi-deps-check.sh
COPY --from=vcontainer-csi-plugin-utils /dest /
COPY --from=builder /build/vcontainer-storage-interface /bin/vcontainer-storage-interface
COPY --from=certs /etc/ssl/certs /etc/ssl/certs

LABEL name="vcontainer-storage-interface" \
      license="Apache Version 2.0" \
      maintainers="Cuong. Duong Manh" \
      email="cuongdm3@vng.com.vn" \
      description="vContainer CSI plugin for Kubernetes"

CMD ["/bin/vcontainer-storage-interface"]

