import collections, json, sys
blocks = {}
for line in open(sys.argv[1]):
    if line.startswith('mode:'): continue
    location, statements, count = line.split()
    previous = blocks.get(location, (int(statements), False))
    assert previous[0] == int(statements)
    blocks[location] = (int(statements), previous[1] or int(count) > 0)
totals = collections.defaultdict(lambda: [0, 0])
for location, (n, covered) in blocks.items():
    filename = location.rsplit(':', 1)[0]
    package = filename.rsplit('/', 1)[0]
    for key in [filename, package, 'TOTAL']:
        totals[key][0] += n * covered
        totals[key][1] += n
print(json.dumps({k: {'covered': c, 'statements': n, 'percent': round(100*c/n, 4) if n else None}
                  for k, (c, n) in sorted(totals.items())}, indent=2))