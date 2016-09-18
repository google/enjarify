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
use std::process::Command;

use strings::*;

use jvm::optimization::options::Options;
use super::{read, read_jar, translate, write_to_jar};

pub fn main() {
    let stubs = read_jar("../tests/stubs/stubs.zip");
    for test in 1..7 {
        println!("test{}", test);
        let data = read(format!("../tests/test{}/classes.dex", test));
        let dexes = vec![data];
        let expected = read(format!("../tests/test{}/expected.txt", test));
        let expected = (to_string(expected) + "\n").replace("\r\n", "\n");

        for opts in &[Options::none(), Options::pretty(), Options::all()] {
            let (mut classes, mut ordkeys, errors) = translate(*opts, &dexes);
            assert!(errors.is_empty());

            classes.extend(stubs.clone());
            ordkeys.extend(stubs.iter().map(|&(ref k,_)| k.clone()));
            write_to_jar("out.jar", classes, ordkeys);

            let output = Command::new("java")
                .args(&["-Xss515m", "-jar", "out.jar", "a.a"])
                .output().expect("failed to execute process");
            assert!(output.stderr.is_empty());
            let result = output.stdout;

            // let mut f = File::create("actual.txt").unwrap();
            // f.write_all(&result).unwrap();

            assert_eq!(to_string(result), expected);
        }
    }
    println!("all tests passed!");
}
