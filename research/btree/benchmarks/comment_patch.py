import sys

p = sys.argv[1]
src = open(p).read()
old = "\tcase descend:\n\t\tif start.valid && !hit {\n"
new = ("\tcase descend:\n"
       "\t\t// As above: once the start has been passed, every item left is\n"
       "\t\t// below it, and a node whose first item is above the stop holds\n"
       "\t\t// no item at or below it.\n"
       "\t\tif start.valid && !hit {\n")
assert src.count(old) == 1
open(p, "w").write(src.replace(old, new))
print("patched", p)
