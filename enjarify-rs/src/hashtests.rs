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

extern crate crypto;
use self::crypto::digest::Digest;
use self::crypto::sha2::Sha256;

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
    let mut fullhash = vec![];

    for test in 1..8 {
        println!("test{}", test);
        let data = read(format!("../tests/test{}/classes.dex", test));
        let dexes = vec![data];
        for bits in 0...255 {
            let (classes, ordkeys, errors) = translate(Options::from(bits), &dexes);
            assert!(errors.is_empty());

            for (_, k) in ordkeys.into_iter().enumerate() {
                let ref cls = classes[&k];
                // println!("filename {}", hexdigest(k.as_bytes()));
                println!("{:08b} {}", bits, hexdigest(&cls));

                // let fname = format!("../../rsout/{}_{}_{}.class", test, bits, i);
                // let mut f = File::create(fname).unwrap();
                // f.write_all(cls).unwrap();

                // fullhash.extend(k.as_bytes());
                fullhash.extend(cls);
                fullhash = hash(&fullhash);
            }
        }
    }

    println!("done!\nFinal hash: {}", hexdigest(&fullhash));
}
