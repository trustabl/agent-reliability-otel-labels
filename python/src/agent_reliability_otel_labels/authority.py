"""Policy bindings: which signed policies were in force.

One EVENT per policy, never a flattened hash and never parallel arrays. A
single combined hash reports ACS as present when only OpenShell loaded, and
index-correlated arrays mispair an id with the wrong hash the moment one is
truncated - in output that gets signed, that is a defect with a long half-life.

Contract identity is deliberately NOT emitted here. The deployment already
stamps it through OTEL_RESOURCE_ATTRIBUTES and the collector processor reads
it there; duplicating it on the span would create two producers for one key.
"""

from dataclasses import dataclass

from agent_reliability_otel_labels import keys
from agent_reliability_otel_labels.spans import _unusable


@dataclass(frozen=True)
class Binding:
    """One signed policy artifact attested on a span.

    `required` says this agent class must carry this rail, so its absence is a
    finding rather than a non-event. `source` records how it arrived;
    `external` marks an artifact this contract did not generate, which is not
    the same as one it did.
    """

    id: str
    type: str
    sha256: str
    version: str
    required: bool
    source: str

    def validate(self) -> None:
        if self.type not in keys.BINDING_TYPES:
            raise ValueError(
                f"binding type must be one of {sorted(keys.BINDING_TYPES)}, "
                f"got {self.type!r}"
            )
        if self.source not in keys.BINDING_SOURCES:
            raise ValueError(
                f"binding source must be one of {sorted(keys.BINDING_SOURCES)}, "
                f"got {self.source!r}"
            )


def bind_authority(span, *, bindings) -> None:
    """Attest every policy in force on this agent or sandbox root.

    A subagent lists its own bindings, or one marked source='inherited'. An
    empty list emits nothing on purpose: a handoff carrying no binding is a
    coverage hole, and making it visible is the point.
    """
    if _unusable(span):
        return

    for binding in bindings:
        binding.validate()

    for binding in bindings:
        span.add_event(keys.POLICY_BINDING_EVENT, {
            keys.BINDING_ID: binding.id,
            keys.BINDING_TYPE: binding.type,
            keys.BINDING_SHA256: binding.sha256,
            keys.BINDING_VERSION: binding.version,
            keys.BINDING_REQUIRED: binding.required,
            keys.BINDING_SOURCE: binding.source,
        })
