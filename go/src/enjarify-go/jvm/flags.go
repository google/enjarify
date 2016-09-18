// Copyright 2015 Google Inc. All Rights Reserved.
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
package jvm

const ACC_PUBLIC = 0x1
const ACC_PRIVATE = 0x2
const ACC_PROTECTED = 0x4
const ACC_STATIC = 0x8
const ACC_FINAL = 0x10
const ACC_SYNCHRONIZED = 0x20
const ACC_VOLATILE = 0x40
const ACC_BRIDGE = 0x40
const ACC_TRANSIENT = 0x80
const ACC_VARARGS = 0x80
const ACC_NATIVE = 0x100
const ACC_INTERFACE = 0x200
const ACC_ABSTRACT = 0x400
const ACC_STRICT = 0x800
const ACC_SYNTHETIC = 0x1000
const ACC_ANNOTATION = 0x2000
const ACC_ENUM = 0x4000
const ACC_CONSTRUCTOR = 0x10000
const ACC_DECLARED_SYNCHRONIZED = 0x20000

// Might as well include this for completeness even though modern JVMs ignore it
const ACC_SUPER = 0x20

const CLASS_FLAGS = ACC_PUBLIC | ACC_FINAL | ACC_SUPER | ACC_INTERFACE | ACC_ABSTRACT | ACC_SYNTHETIC | ACC_ANNOTATION | ACC_ENUM
const FIELD_FLAGS = ACC_PUBLIC | ACC_PRIVATE | ACC_PROTECTED | ACC_STATIC | ACC_FINAL | ACC_VOLATILE | ACC_TRANSIENT | ACC_SYNTHETIC | ACC_ENUM
const METHOD_FLAGS = ACC_PUBLIC | ACC_PRIVATE | ACC_PROTECTED | ACC_STATIC | ACC_FINAL | ACC_SYNCHRONIZED | ACC_BRIDGE | ACC_VARARGS | ACC_NATIVE | ACC_ABSTRACT | ACC_STRICT | ACC_SYNTHETIC
