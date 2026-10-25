import sys

# Temporary etcd change for measurement: unsafeVisit uses AscendRange when an
# end key is given, so the end check runs once per node instead of once per
# item. Restored from a saved copy after the run.
p = sys.argv[1]
src = open(p).read()
old = ("func (ti *treeIndex) unsafeVisit(key, end []byte, f func(ki *keyIndex) bool) {\n"
       "\tkeyi, endi := &keyIndex{key: key}, &keyIndex{key: end}\n"
       "\n"
       "\tti.tree.AscendGreaterOrEqual(keyi, func(item *keyIndex) bool {\n"
       "\t\tif len(endi.key) > 0 && !item.Less(endi) {\n"
       "\t\t\treturn false\n"
       "\t\t}\n"
       "\t\tif !f(item) {\n"
       "\t\t\treturn false\n"
       "\t\t}\n"
       "\t\treturn true\n"
       "\t})\n"
       "}\n")
new = ("func (ti *treeIndex) unsafeVisit(key, end []byte, f func(ki *keyIndex) bool) {\n"
       "\tkeyi := &keyIndex{key: key}\n"
       "\tif len(end) == 0 {\n"
       "\t\tti.tree.AscendGreaterOrEqual(keyi, f)\n"
       "\t\treturn\n"
       "\t}\n"
       "\tti.tree.AscendRange(keyi, &keyIndex{key: end}, f)\n"
       "}\n")
assert src.count(old) == 1
open(p, "w").write(src.replace(old, new))
print("patched", p)
