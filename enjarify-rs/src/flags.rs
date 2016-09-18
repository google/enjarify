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
pub const ACC_PUBLIC: u16 = 0x1;
pub const ACC_PRIVATE: u16 = 0x2;
pub const ACC_PROTECTED: u16 = 0x4;
pub const ACC_STATIC: u16 = 0x8;
pub const ACC_FINAL: u16 = 0x10;
pub const ACC_SYNCHRONIZED: u16 = 0x20;
pub const ACC_VOLATILE: u16 = 0x40;
pub const ACC_BRIDGE: u16 = 0x40;
pub const ACC_TRANSIENT: u16 = 0x80;
pub const ACC_VARARGS: u16 = 0x80;
pub const ACC_NATIVE: u16 = 0x100;
pub const ACC_INTERFACE: u16 = 0x200;
pub const ACC_ABSTRACT: u16 = 0x400;
pub const ACC_STRICT: u16 = 0x800;
pub const ACC_SYNTHETIC: u16 = 0x1000;
pub const ACC_ANNOTATION: u16 = 0x2000;
pub const ACC_ENUM: u16 = 0x4000;
// pub const ACC_CONSTRUCTOR: u16 = 0x10000;
// pub const ACC_DECLARED_SYNCHRONIZED: u16 = 0x20000;

// Might as well include this for completeness even though modern JVMs ignore it;
pub const ACC_SUPER: u16 = 0x20;

pub const CLASS_FLAGS: u16 = ACC_PUBLIC | ACC_FINAL | ACC_SUPER | ACC_INTERFACE | ACC_ABSTRACT | ACC_SYNTHETIC | ACC_ANNOTATION | ACC_ENUM;
pub const FIELD_FLAGS: u16 = ACC_PUBLIC | ACC_PRIVATE | ACC_PROTECTED | ACC_STATIC | ACC_FINAL | ACC_VOLATILE | ACC_TRANSIENT | ACC_SYNTHETIC | ACC_ENUM;
pub const METHOD_FLAGS: u16 = ACC_PUBLIC | ACC_PRIVATE | ACC_PROTECTED | ACC_STATIC | ACC_FINAL | ACC_SYNCHRONIZED | ACC_BRIDGE | ACC_VARARGS | ACC_NATIVE | ACC_ABSTRACT | ACC_STRICT | ACC_SYNTHETIC;
