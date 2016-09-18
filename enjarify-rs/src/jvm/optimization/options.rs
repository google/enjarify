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
#[derive(Clone, Copy, PartialEq, Eq)]
pub struct Options{
    pub inline_consts: bool,
    pub prune_store_loads: bool,
    pub copy_propagation: bool,
    pub remove_unused_regs: bool,
    pub dup2ize: bool,
    pub sort_registers: bool,
    pub split_pool: bool,
    pub delay_consts: bool,
}
impl Options {
    pub fn from(bits: u8) -> Options { Options{
        inline_consts: bits & (1 << 0) != 0,
        prune_store_loads: bits & (1 << 1) != 0,
        copy_propagation: bits & (1 << 2) != 0,
        remove_unused_regs: bits & (1 << 3) != 0,
        dup2ize: bits & (1 << 4) != 0,
        sort_registers: bits & (1 << 5) != 0,
        split_pool: bits & (1 << 6) != 0,
        delay_consts: bits & (1 << 7) != 0,
    } }
    pub fn none() -> Options { Options::from(0) }
    pub fn all() -> Options { Options::from(255) }
    pub fn pretty() -> Options { Options{
        inline_consts: true,
        prune_store_loads: true,
        copy_propagation: true,
        remove_unused_regs: true,
        ..Options::none()
    } }
}
