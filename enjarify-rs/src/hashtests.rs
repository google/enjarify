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
// use std::io::prelude::*;
// use std::fs::File;
use std::sync::Arc;

extern crate crypto;
use self::crypto::digest::Digest;
use self::crypto::sha2::Sha256;

use futures::Future;
use futures_cpupool::CpuPool;

use strings::*;
use jvm::optimization::options::Options;
use super::{read, translate};

fn hash(s: &bstr) -> BString {
    let mut res = vec![0; 32];
    let mut h = Sha256::new();
    h.input(s);
    h.result(&mut res);
    res
}

fn hexdigest(s: &bstr) -> String {
    let mut h = Sha256::new();
    h.input(s);
    h.result_str()
}

pub fn main() {
    let pool = CpuPool::new_num_cpus();
    let testfiles = (1..8).map(|test| read(format!("../tests/test{}/classes.dex", test))).collect::<Vec<_>>();
    let testfiles = Arc::new(testfiles);

    let output_futures: Vec<_> = (0..(7*256)).into_iter().map(|ind| {
        let testfiles = testfiles.clone();

        pool.spawn_fn(move || {
            let ti = (ind / 256) as usize;
            let bits = ind % 256;
            let dexes = &testfiles[ti..ti+1];

            let pool = CpuPool::new(1);
            let results = translate(&pool, Options::from(bits as u8), dexes);
            let output = results.into_iter().map(|(_, res)| {
                let cls = res.unwrap();
                let digest = hexdigest(&cls);
                (cls, digest)
            }).collect();

            let res: Result<Vec<_>, ()> = Ok(output);
            res
        })
    }).collect();

    let mut fullhash = vec![];
    for (ind, fut) in output_futures.into_iter().enumerate() {
        let bits = ind % 256;
        if bits==0 {println!("test{}", (ind/256)+1);}

        let pairs = fut.wait().unwrap();
        for (cls, digest) in pairs {
            println!("{:08b} {}", bits, digest);
            fullhash.extend(cls);
            fullhash = hash(&fullhash);
        }
    }
    println!("done!\nFinal hash: {}", hexdigest(&fullhash));
}
