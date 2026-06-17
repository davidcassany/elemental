ARG ELEMENTAL_IMAGE=elemental-image:latest
ARG SOURCE_IMAGE=registry.suse.com/elemental/base-os-kernel-default:16.0

FROM ${ELEMENTAL_IMAGE} AS elemental

FROM ${SOURCE_IMAGE}

COPY --from=elemental /usr/bin/elemental3ctl /usr/bin/elemental3ctl
COPY pkg/dracut/modules.d/29elemental-platform-resolver /usr/lib/dracut/modules.d/29elemental-platform-resolver

RUN chmod 0755 \
    /usr/lib/dracut/modules.d/29elemental-platform-resolver/module-setup.sh \
    /usr/lib/dracut/modules.d/29elemental-platform-resolver/elemental-platform-resolver.sh && \
    if command -v dracut >/dev/null 2>&1; then dracut -f --regenerate-all; fi
