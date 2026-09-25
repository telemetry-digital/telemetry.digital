# Adds the library sources to the include path and turns the Arduino example sketch into a .cpp for the CI build.
Import("env")
import os
root = os.path.abspath(os.path.join(env.subst("$PROJECT_DIR"), ".."))
env.Append(CPPPATH=[os.path.join(root, "src"), os.path.join(root, "ports", "arduino")])
gen = os.path.join(root, "ci", "gen")
os.makedirs(gen, exist_ok=True)
ino = os.path.join(root, "examples", "arduino", "fridge_node", "fridge_node.ino")
with open(ino, encoding="utf-8") as f:
    body = f.read()
with open(os.path.join(gen, "fridge_node.cpp"), "w", encoding="utf-8") as f:
    f.write("#include <Arduino.h>\n" + body)
