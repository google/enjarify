// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
use std::collections::HashMap;

use jvm::constantpool::ConstantPool;
use jvm::constantpool::Entry;
use jvm::ir;
use jvm::writeir::IRWriter;

pub fn allocate_required_constants(pool: &mut ConstantPool, irs: Vec<&IRWriter>) {
    // We allocate the constants pretty much greedily. This is far from optimal,
    // but it shouldn't be a big deal since this code is almost never required
    // in the first place. In fact, there are no known real world classes that
    // even come close to exhausting the constant pool.
    if irs.is_empty() { return; }

    let mut narrow_pairs = HashMap::new();
    let mut wide_pairs = HashMap::new();
    let mut alt_lens = HashMap::new();
    for irdata in irs {
        for ins in irdata.instructions.iter() {
            if let ir::PrimConstant(ref data) = ins.sub {
                let len = ins.bytecode.as_ref().unwrap().len();
                alt_lens.insert(data.key.clone(), len);
                if data.st.is_wide() {
                    if len > 3 { *wide_pairs.entry(data.key.clone()).or_insert(0) += 1; }
                } else {
                    if len > 2 { *narrow_pairs.entry(data.key.clone()).or_insert(0) += 1; }
                }
            }
        }
    }

    // see if already in the constant pool
    for x in pool.vals() { if let Some(x) = x.as_ref() {
        narrow_pairs.remove(&x);
        wide_pairs.remove(&x);
    }}

    // if we have enough space for all required constants, preferentially allocate
    // most commonly used constants to first 255 slots
    if pool.space() >= narrow_pairs.len() + 2*wide_pairs.len() && pool.lowspace() > 0 {
        let most_common: Vec<Entry> = {
            let mut most_common: Vec<_> = narrow_pairs.iter().collect();
            most_common.sort_by_key(|&(ref p, &count)| (-(count as i64), p.cmp_key()));
            most_common.into_iter().take(pool.lowspace()).map(|(ref p, _count)| (*p).clone()).collect()
        };
        for k in most_common.into_iter() {
            narrow_pairs.remove(&k);
            pool.insert_directly(k, true);
        }
    }

    let mut scores = HashMap::new();
    for (p, count) in narrow_pairs.iter() {
        scores.insert(p.clone(), (alt_lens[p] - 3) * count);
    }
    for (p, count) in wide_pairs.iter() {
        scores.insert(p.clone(), (alt_lens[p] - 3) * count);
    }


    // sort by score
    let mut narrowq: Vec<_> = {
        let mut items: Vec<Entry> = narrow_pairs.into_iter().map(|(p, _)| p).collect();
        items.sort_by_key(|p| (scores[p], p.cmp_key()));
        items
    };
    let mut wideq: Vec<_> = {
        let mut items: Vec<Entry> = wide_pairs.into_iter().map(|(p, _)| p).collect();
        items.sort_by_key(|p| (scores[p], p.cmp_key()));
        items
    };

    while pool.space() >= 1 && (!narrowq.is_empty() || !wideq.is_empty()) {
        if narrowq.is_empty() && pool.space() < 2 { break; }

        let nscore = match narrowq.len() {
            0 => 0,
            1 => scores[&narrowq[0]],
            _ => scores[&narrowq[narrowq.len()-1]] + scores[&narrowq[narrowq.len()-2]],
        };
        let wscore = if wideq.is_empty() {0} else {scores[&wideq[wideq.len()-1]]};

        if pool.space() >= 2 && wscore > nscore && wscore > 0 {
            pool.insert_directly(wideq.pop().unwrap(), false);
        } else if nscore > 0 {
            pool.insert_directly(narrowq.pop().unwrap(), true);
        } else {
            break;
        }
    }
}
