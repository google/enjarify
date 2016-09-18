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
use std::ops::Deref;
use std::rc::Rc;


// This class represents a list as a persistent n-ary tree
// This has much slower access and updates than a real list but has the advantage
// of sharing memory with previous versions of the list when only a few elements
// are changed. See http://en.wikipedia.org/wiki/Persistent_data_structure#Trees
// Also, default values are not stored, so this is good for sparse arrays

// The first SIZE elements are stored directly, the rest are stored in one of SPLIT subtrees
const SIZE: usize = 16;
const SPLIT: usize = 16;

fn clone<T: Clone>(src: &[T; 16]) -> [T; 16] {
    [src[0].clone(), src[1].clone(), src[2].clone(), src[3].clone(), src[4].clone(), src[5].clone(), src[6].clone(), src[7].clone(), src[8].clone(), src[9].clone(), src[10].clone(), src[11].clone(), src[12].clone(), src[13].clone(), src[14].clone(), src[15].clone()]
}

#[derive(Default)]
struct TreeNode<T> {
    direct: [T; SIZE],
    children: [TreePtr<T>; SPLIT],
}
impl<T: Clone + Default> Clone for TreeNode<T> {
    fn clone(&self) -> Self { TreeNode{ direct: clone(&self.direct), children: clone(&self.children) } }
}
impl<T: Clone + Default + Eq> TreeNode<T> {
    fn get(&self, mut i: usize) -> T {
        if i < SIZE { return self.direct[i].clone(); }
        i -= SIZE;
        self.children[i % SPLIT]._get(i / SPLIT)
    }

    fn set(&mut self, mut i: usize, val: T) {
        if i < SIZE { self.direct[i] = val; return; }
        i -= SIZE;
        self.children[i % SPLIT]._set(i / SPLIT, val);
    }

    fn merge<F>(&mut self, rhs: &Self, func: &F, def_is_bot: bool) -> bool
        where F : Fn(T, T) -> T
    {
        let mut result = false;
        for i in 0..SIZE {
            let merged = func(self.direct[i].clone(), rhs.direct[i].clone());
            if merged != self.direct[i] {
                self.direct[i] = merged;
                result = true;
            }
        }
        for i in 0..SPLIT {
            result = result | self.children[i].merge(&rhs.children[i], func, def_is_bot);
        }
        result
    }
}

#[derive(Clone, Default)]
pub struct TreePtr<T>(Option<Rc<TreeNode<T>>>);
impl<T: Clone + Default + Eq> TreePtr<T> {
    fn _get(&self, i: usize) -> T {
        match self.0 { Some(ref node) => node.get(i), None => T::default() }
    }
    pub fn get(&self, i: u16) -> T { self._get(i as usize) }

    fn ensure_mut(&mut self) -> &mut TreeNode<T> {
        if self.0.is_none() {
            self.0 = Some(Rc::new(TreeNode::default()));
        }
        else if Rc::get_mut(self.0.as_mut().unwrap()).is_none() {
            let node = self.0.as_ref().unwrap().deref().clone();
            self.0 = Some(Rc::new(node));
        }
        Rc::get_mut(self.0.as_mut().unwrap()).unwrap()
    }

    fn _set(&mut self, i: usize, val: T) {
        if val == self._get(i) { return; }
        self.ensure_mut().set(i, val);
    }
    pub fn set(&mut self, i: u16, val: T) { self._set(i as usize, val) }

    pub fn is(&self, rhs: &Self) -> bool {
        match (self.0.as_ref(), rhs.0.as_ref()) {
            (None, None) => true,
            (Some(r1), Some(r2)) => r1 as *const _ == r2 as *const _,
            _ => false
        }
    }

    pub fn merge<F>(&mut self, rhs: &Self, func: &F, def_is_bot: bool) -> bool
        where F : Fn(T, T) -> T
    {
        if self.is(rhs) { return false; }

        if def_is_bot {
            if self.0.is_none() { return false; }
            if rhs.0.is_none() { self.0 = None; return true; }
        } else { // default is top
            if rhs.0.is_none() { return false; }
            if self.0.is_none() { self.0 = rhs.0.clone(); return true; }
        }

        self.ensure_mut().merge(rhs.0.as_ref().unwrap().deref(), func, def_is_bot)
    }
}
