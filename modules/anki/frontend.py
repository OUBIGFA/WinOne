"""Narrow original reviewer protobuf callbacks, scoped to the current card."""
import base64
from anki import frontend_pb2


def invoke(engine, params):
    gui = engine.runtime.gui
    if not gui.card or params["key"] != gui.mutation_key:
        raise ValueError("Review card has changed")
    data = base64.b64decode(params.get("data", ""), validate=True)
    if params["method"] == "getSchedulingStatesWithContext":
        result = frontend_pb2.SchedulingStatesWithContext(states=gui.states, context=gui.scheduling_context)
        return base64.b64encode(result.SerializeToString()).decode()
    if params["method"] == "setSchedulingStates":
        value = frontend_pb2.SetSchedulingStatesRequest.FromString(data)
        if value.key != gui.mutation_key or value.states.current != gui.states.current:
            raise ValueError("Custom scheduling state does not belong to this card")
        gui.states.CopyFrom(value.states)
        gui.custom_applied = True
        labels = engine.col.sched.describe_next_states(gui.states)
        for answer, label in zip(gui.prediction["answers"], labels):
            answer["label"] = label
        return ""
    raise ValueError("Unknown card renderer callback")
