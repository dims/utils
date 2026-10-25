import sys

# Applies only the iterate changes to an unmodified btree.go, leaving find
# and get as upstream has them.
p = sys.argv[1]
src = open(p).read()


def rep(old, new):
    global src
    c = src.count(old)
    assert c == 1, (c, old[:70])
    src = src.replace(old, new)


rep("\tcase ascend:\n"
    "\t\tif start.valid {\n"
    "\t\t\tindex, _ = n.items.find(start.item, n.cow.less)\n"
    "\t\t}\n"
    "\t\tfor i := index; i < len(n.items); i++ {\n",
    "\tcase ascend:\n"
    "\t\t// Past the start, every item is greater than it: nothing to find.\n"
    "\t\tif start.valid && !hit {\n"
    "\t\t\tindex, _ = n.items.find(start.item, n.cow.less)\n"
    "\t\t}\n"
    "\t\t// If the last item is below the stop, every item is.\n"
    "\t\tallBelowStop := !stop.valid || len(n.items) == 0 || n.cow.less(n.items[len(n.items)-1], stop.item)\n"
    "\t\tfor i := index; i < len(n.items); i++ {\n")
rep("\t\t\thit = true\n"
    "\t\t\tif stop.valid && !n.cow.less(n.items[i], stop.item) {\n"
    "\t\t\t\treturn hit, false\n"
    "\t\t\t}\n",
    "\t\t\thit = true\n"
    "\t\t\tif !allBelowStop && !n.cow.less(n.items[i], stop.item) {\n"
    "\t\t\t\treturn hit, false\n"
    "\t\t\t}\n")
rep("\tcase descend:\n"
    "\t\tif start.valid {\n"
    "\t\t\tindex, found = n.items.find(start.item, n.cow.less)\n"
    "\t\t\tif !found {\n"
    "\t\t\t\tindex = index - 1\n"
    "\t\t\t}\n"
    "\t\t} else {\n"
    "\t\t\tindex = len(n.items) - 1\n"
    "\t\t}\n",
    "\tcase descend:\n"
    "\t\t// Mirror of the ascend case.\n"
    "\t\tif start.valid && !hit {\n"
    "\t\t\tindex, found = n.items.find(start.item, n.cow.less)\n"
    "\t\t\tif !found {\n"
    "\t\t\t\tindex = index - 1\n"
    "\t\t\t}\n"
    "\t\t} else {\n"
    "\t\t\tindex = len(n.items) - 1\n"
    "\t\t}\n"
    "\t\tallAboveStop := !stop.valid || len(n.items) == 0 || n.cow.less(stop.item, n.items[0])\n")
rep("\t\t\tif stop.valid && !n.cow.less(stop.item, n.items[i]) {\n"
    "\t\t\t\treturn hit, false //\tcontinue\n"
    "\t\t\t}\n",
    "\t\t\tif !allAboveStop && !n.cow.less(stop.item, n.items[i]) {\n"
    "\t\t\t\treturn hit, false\n"
    "\t\t\t}\n")
open(p, "w").write(src)
print("patched", p)
