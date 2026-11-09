"""Load the pinned AnkiConnect domain code under the WinOne host.

The upstream package entry point installs Qt hooks. Native WinOne owns those
hooks and windows, so only the unchanged domain modules are imported here.
"""
import ast
import sys
import types
from pathlib import Path


def load(directory: Path):
    package = types.ModuleType("winone_ankiconnect")
    package.__path__ = [str(directory)]
    sys.modules[package.__name__] = package
    # The sole Qt dependency in utility settings is replaced by the host's
    # settings provider; the original source and its defaults remain intact.
    # Defaults containing os.getenv cannot be literal-evaluated. Use explicit
    # host defaults; network serving and origin permissions belong to the Host.
    settings = {"apiVersion": 6, "apiKey": None, "webTimeout": 10000}
    util = types.ModuleType(package.__name__ + ".util")
    util.setting = lambda key: settings[key]
    sys.modules[util.__name__] = util
    methods = {}
    tree = ast.parse((directory / "__init__.py").read_text(encoding="utf-8"))
    for node in ast.walk(tree):
        if not isinstance(node, ast.FunctionDef):
            continue
        if not any((isinstance(d, ast.Call) and isinstance(d.func, ast.Attribute) and d.func.attr == "api")
                   or (isinstance(d, ast.Attribute) and d.attr == "api") for d in node.decorator_list):
            continue
        names = [a.arg for a in node.args.args if a.arg != "self"]
        optional = dict(zip(names[len(names)-len(node.args.defaults):],
                            [ast.literal_eval(d) for d in node.args.defaults]))
        methods[node.name] = (names, optional)
    return methods
