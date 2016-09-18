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
use std::panic;

pub struct ClassfileLimitExceeded;
pub fn classfile_limit_exceeded() -> ! {panic!(ClassfileLimitExceeded);}

pub fn set_hook() {
    println!("setting panic hook");
    let old = panic::take_hook();
    panic::set_hook(Box::new(move |info: &panic::PanicInfo| {
        if !info.payload().is::<ClassfileLimitExceeded>() { old(info); }
        // else { println!("CFLE thrown"); }
    })
    );
}
