import sys
p = sys.argv[1]; src = open(p).read()
def rep(old, new):
    global src
    assert src.count(old) == 1, old[:50]
    src = src.replace(old, new)
rep("package btree\n", "package btreefind\n")
rep('\t"sort"\n', '')
rep("\ti := sort.Search(len(s), func(i int) bool {\n\t\treturn less(item, s[i])\n\t})\n\tif i > 0 && !less(s[i-1], item) {\n\t\treturn i - 1, true\n\t}\n\treturn i, false\n",
    "\tlo, hi := 0, len(s)\n\tfor lo < hi {\n\t\th := int(uint(lo+hi) >> 1)\n\t\tif !less(item, s[h]) {\n\t\t\tlo = h + 1\n\t\t} else {\n\t\t\thi = h\n\t\t}\n\t}\n\tif lo > 0 && !less(s[lo-1], item) {\n\t\treturn lo - 1, true\n\t}\n\treturn lo, false\n")
open(p, "w").write(src)
