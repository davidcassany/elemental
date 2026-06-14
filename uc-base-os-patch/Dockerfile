ARG ELEMENTAL_IMAGE=elemental-image:latest
ARG SOURCE_IMAGE=registry.suse.com/elemental/base-os-kernel-default:16.0

FROM ${ELEMENTAL_IMAGE} AS elemental

FROM ${SOURCE_IMAGE}

COPY --from=elemental /usr/bin/elemental3ctl /usr/bin/elemental3ctl
