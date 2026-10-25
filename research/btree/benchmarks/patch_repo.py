import sys

# Applies the search and iteration changes to the fork's btree.go in place.
# Every pattern must match exactly once, or nothing is written.
p = sys.argv[1]
src = open(p).read()


def rep(old, new):
    global src
    c = src.count(old)
    assert c == 1, (c, old[:70])
    src = src.replace(old, new)


rep('\t"sort"\n', '')

# find: a plain binary search with sort.Search's contract. sort.Search takes
# a closure, so each probe was an indirect call into a closure that made a
# second indirect call into less.
rep("\ti := sort.Search(len(s), func(i int) bool {\n"
    "\t\treturn less(item, s[i])\n"
    "\t})\n"
    "\tif i > 0 && !less(s[i-1], item) {\n"
    "\t\treturn i - 1, true\n"
    "\t}\n"
    "\treturn i, false\n",
    "\tlo, hi := 0, len(s)\n"
    "\tfor lo < hi {\n"
    "\t\th := int(uint(lo+hi) >> 1)\n"
    "\t\tif !less(item, s[h]) {\n"
    "\t\t\tlo = h + 1\n"
    "\t\t} else {\n"
    "\t\t\thi = h\n"
    "\t\t}\n"
    "\t}\n"
    "\tif lo > 0 && !less(s[lo-1], item) {\n"
    "\t\treturn lo - 1, true\n"
    "\t}\n"
    "\treturn lo, false\n")

# get: descend in a loop instead of recursing.
rep("func (n *node[T]) get(key T) (_ T, _ bool) {\n"
    "\ti, found := n.items.find(key, n.cow.less)\n"
    "\tif found {\n"
    "\t\treturn n.items[i], true\n"
    "\t} else if len(n.children) > 0 {\n"
    "\t\treturn n.children[i].get(key)\n"
    "\t}\n"
    "\treturn\n"
    "}\n",
    "func (n *node[T]) get(key T) (_ T, _ bool) {\n"
    "\tfor {\n"
    "\t\ti, found := n.items.find(key, n.cow.less)\n"
    "\t\tif found {\n"
    "\t\t\treturn n.items[i], true\n"
    "\t\t}\n"
    "\t\tif len(n.children) == 0 {\n"
    "\t\t\treturn\n"
    "\t\t}\n"
    "\t\tn = n.children[i]\n"
    "\t}\n"
    "}\n")

# iterate, ascending: search for the start only until it has been passed,
# and test the stop bound once per node when the whole node lies below it.
rep("\tcase ascend:\n"
    "\t\tif start.valid {\n"
    "\t\t\tindex, _ = n.items.find(start.item, n.cow.less)\n"
    "\t\t}\n"
    "\t\tfor i := index; i < len(n.items); i++ {\n",
    "\tcase ascend:\n"
    "\t\t// Once the start has been passed, every item of every node visited\n"
    "\t\t// after it is greater than the start, so there is nothing to find.\n"
    "\t\tif start.valid && !hit {\n"
    "\t\t\tindex, _ = n.items.find(start.item, n.cow.less)\n"
    "\t\t}\n"
    "\t\t// When the node's last item is below the stop, so is every item, and\n"
    "\t\t// the per-item stop check can be skipped.\n"
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

# iterate, descending: the same two changes.
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
