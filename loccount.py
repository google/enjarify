# Copyright 2016 Google Inc. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import sys, collections, os
target = sys.argv[1]

lines = collections.defaultdict(int)
chars = collections.defaultdict(int)

for root, dirs, files in os.walk(target):
    for fname in files:
        path = fname = os.path.join(root, fname)
        fname, _, ext = fname.rpartition('.')
        if ext not in 'py go rs'.split():
            continue

        with open(path, 'r') as f:
            source = f.read()

        comment = '#' if ext == 'py' else '//'
        assert not fname.endswith('/')

        for line in source.splitlines():
            line = line.strip()
            if not line or line.startswith('#') or line.startswith('//'):
                continue

            line, _, _ = line.partition(comment)
            line = line.strip()
            assert line
            lines[fname] += 1
            chars[fname] += len(line)

for k in list(lines):
    folder, sep, fname = k.rpartition('/')
    if fname.startswith('gen'):
        out = folder + sep + fname[3:]
        print('dropping', out)
        lines.pop(out, None)
        chars.pop(out, None)

print('{} files {} lines {} chars'.format(len(lines), sum(lines.values()), sum(chars.values())))

