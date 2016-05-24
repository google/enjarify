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
.class public La/a;
.super Landroid/app/Activity;

.field public static c:L0;
.field public static c:L1;
.field public static c:L00;
.field public static c:L01;

.field public static w:Ljava/lang/Throwable;
.field public static w:L0;
.field public static w:L1;
.field public static w:L00;
.field public static w:L01;

.field public static w:[L0;

.method public static testTypesSub(Z)V
    .locals 06

    sget-object v0, La/a;->c:L0;
    sget-object v1, La/a;->c:L1;

    if-eqz p0, :else
        sget-object v0, La/a;->c:L00;
        sget-object v1, La/a;->c:L01;
        sget-object v2, La/a;->c:L0;
        sget-object v3, La/a;->c:L0;
        sget-object v4, La/a;->c:L00;
        sget-object v5, La/a;->c:L01;
    goto :end
:else
        sget-object v0, La/a;->c:L01;
        sget-object v1, La/a;->c:L0;
        sget-object v2, La/a;->c:L00;
        sget-object v3, La/a;->c:L1;
        sget-object v4, La/a;->c:L00;
        sget-object v5, La/a;->c:L01;
:end

    invoke-static {v0}, LL/util;->print(Ljava/lang/Object;)V
    invoke-static {v1}, LL/util;->print(Ljava/lang/Object;)V
    invoke-static {v2}, LL/util;->print(Ljava/lang/Object;)V
    invoke-static {v3}, LL/util;->print(Ljava/lang/Object;)V
    invoke-static {v4}, LL/util;->print(Ljava/lang/Object;)V
    invoke-static {v5}, LL/util;->print(Ljava/lang/Object;)V

    sput-object v0, La/a;->w:L0;
    sput-object v1, La/a;->w:L0;
    sput-object v2, La/a;->w:L0;
    sput-object v3, La/a;->w:Ljava/lang/Throwable;
    sput-object v4, La/a;->w:Ljava/lang/Throwable;
    sput-object v5, La/a;->w:L01;

    return-void
.end method

.method public static testArrayTypesSub(ZZ)V
    .locals 06

    sget-object v0, La/a;->c:L0;
    sget-object v1, La/a;->c:L1;

    if-eqz p0, :else
        sget-object v0, La/a;->c:L00;
        sget-object v1, La/a;->c:L01;
        sget-object v4, La/a;->c:L01;
    goto :end
:else
        sget-object v0, La/a;->c:L01;
        sget-object v1, La/a;->c:L0;
        sget-object v4, La/a;->c:L00;
:end

    if-eqz p1, :elseb
        sget-object v2, La/a;->c:L0;
        sget-object v3, La/a;->c:L0;
        sget-object v5, La/a;->c:L01;
    goto :endb
:elseb
        sget-object v2, La/a;->c:L00;
        sget-object v3, La/a;->c:L1;
        sget-object v5, La/a;->c:L00;
:endb
    move-object v5, v4

    instance-of v0, v1, L01;
    if-eqz v0, :else2
        filled-new-array {v1}, [L01;
        move-result-object v0
    goto :end2
:else2
        filled-new-array {v1, v2}, [L0;
        move-result-object v0
:end2
    invoke-static {v0}, LL/util;->print(Ljava/lang/Object;)V
    sput-object v0, La/a;->w:[L0;

    const/16 v1, 0
    aput-object v4, v0, v1


    instance-of v1, v3, L0;
    if-eqz v1, :else3
        move-object v5, v3
    goto :end3
:else3
:end3

    const/16 v1, 0
:try1
    aput-object v5, v0, v1
:try1e
    goto :handlere

    .catchall {:try1 .. :try1e} :handler
:handler
    move-exception v0
:handlere

    invoke-static {v0}, LL/util;->print(Ljava/lang/Object;)V
    return-void
.end method

.method public static testFilledArray(Z)V
    .locals 06

    if-eqz p0, :else
        sget-object v0, La/a;->c:L00;
        check-cast v0, L0;
    goto :end
:else
        sget-object v0, La/a;->c:L01;
:end

    filled-new-array {v0}, [L0;
    move-result-object v1
    invoke-static {v1}, LL/util;->print(Ljava/lang/Object;)V

    move-object v3, v0
    move-object v1, v0
    instance-of v2, v1, L01;
    if-eqz v2, :end2

        filled-new-array {v3}, [L0;
        move-result-object v1
        invoke-static {v1}, LL/util;->print(Ljava/lang/Object;)V

        filled-new-array {v0}, [L01;
        move-result-object v1
        invoke-static {v1}, LL/util;->print(Ljava/lang/Object;)V
:end2

    filled-new-array {v0, v1}, [Ljava/lang/Object;
    move-result-object v1
    invoke-static {v1}, LL/util;->print(Ljava/lang/Object;)V

    return-void
.end method

.method public static testArrayGet(Z)V
    .locals 06

    if-eqz p0, :else
        sget-object v0, La/a;->c:L00;
        filled-new-array {v0, v0}, [L00;
        move-result-object v1
    goto :end
:else
        sget-object v0, La/a;->c:L1;
        sget-object v1, La/a;->c:L1;
:end

    instance-of v2, v1, [L0;
    if-eqz v2, :end2

        aget-object v2, v1, v2
        sput-object v2, La/a;->w:L0;
        check-cast v2, L00;
        sput-object v2, La/a;->w:L00;
:end2

    invoke-static {v0}, LL/util;->print(Ljava/lang/Object;)V
    invoke-static {v1}, LL/util;->print(Ljava/lang/Object;)V

:try1
    throw v0
    return-void
:try1e
    .catchall {:try1 .. :try1e} :handler
:handler
    return-void
    return-void
    return-void
.end method


.method public static testTypes()V
    .locals 04
    const-string v0, "testTypes"
    invoke-static {v0}, LL/util;->print(Ljava/lang/Object;)V

    const v0, 0
    invoke-static {v0}, La/a;->testTypesSub(Z)V
    invoke-static {v0}, La/a;->testFilledArray(Z)V
    invoke-static {v0}, La/a;->testArrayGet(Z)V
    const v1, 0
    invoke-static {v0, v1}, La/a;->testArrayTypesSub(ZZ)V
    const v1, 1
    invoke-static {v0, v1}, La/a;->testArrayTypesSub(ZZ)V


    const v0, 1
    invoke-static {v0}, La/a;->testTypesSub(Z)V
    invoke-static {v0}, La/a;->testFilledArray(Z)V
    invoke-static {v0}, La/a;->testArrayGet(Z)V
    const v1, 0
    invoke-static {v0, v1}, La/a;->testArrayTypesSub(ZZ)V
    const v1, 1
    invoke-static {v0, v1}, La/a;->testArrayTypesSub(ZZ)V

    return-void
.end method

.method public static _init_()V
    .locals 04
    const-string v0, "_init_"
    invoke-static {v0}, LL/util;->print(Ljava/lang/Object;)V

    new-instance v0, L0;
    invoke-direct {v0}, L0;-><init>()V
    sput-object v0, La/a;->c:L0;

    new-instance v0, L1;
    invoke-direct {v0}, L1;-><init>()V
    sput-object v0, La/a;->c:L1;

    new-instance v0, L00;
    invoke-direct {v0}, L00;-><init>()V
    sput-object v0, La/a;->c:L00;

    new-instance v0, L01;
    invoke-direct {v0}, L01;-><init>()V
    sput-object v0, La/a;->c:L01;
    return-void
.end method

.method public static testBoolArraysSub(Z)V
    .locals 07
    const v0, 0
    const v1, 1

    if-eqz p0, :else
        move v2, v0
        move v3, v1
    goto :end
:else
        move v2, v1
        move v3, v0
        const v0, -1
:end


    const v4, 6
    filled-new-array/range {v0 .. v4}, [I
    move-result-object v6

    new-array v5, v4, [B
    new-array v4, v4, [Z

    aput-boolean v1, v4, v1
    aput-byte v1, v5, v1
    add-int/lit8 v1, v1, -1

    aput-boolean v0, v4, v1
    aput-byte v0, v5, v1
    add-int/lit8 v1, v1, 2

    aput-boolean v2, v4, v1
    aput-byte v2, v5, v1
    add-int/lit8 v1, v1, 1

    aput-boolean v3, v4, v1
    aput-byte v3, v5, v1

    filled-new-array {v6, v5, v4}, [Ljava/lang/Cloneable;
    move-result-object v6
    invoke-static {v6}, LL/util;->print(Ljava/lang/Object;)V

    fill-array-data v5, :data1
    fill-array-data v4, :data1
    invoke-static {v6}, LL/util;->print(Ljava/lang/Object;)V

    return-void

:data1
    .array-data 1
        1t true 0t -14t
    .end array-data

.end method



.method public static testBoolArrays()V
    .locals 04
    const-string v0, "testBoolArrays"

    const v0, 0
    invoke-static {v0}, La/a;->testBoolArraysSub(Z)V
    const v1, 0
    invoke-static {v0}, La/a;->testBoolArraysSub(Z)V

    return-void
.end method


.method public onCreate(Landroid/os/Bundle;)V
    .locals 12
    move-object/from16 v10, p0
    move-object/from16 v11, p1
    invoke-super {v10, v11}, Landroid/app/Activity;->onCreate(Landroid/os/Bundle;)V

    invoke-static {}, La/a;->_init_()V
    invoke-static {}, La/a;->testTypes()V
    invoke-static {}, La/a;->testBoolArrays()V

    return-void
.end method


.method public constructor <init>()V
    .locals 0
    invoke-direct {p0}, Landroid/app/Activity;-><init>()V
    return-void
.end method

# Android code with no Java equivalent
.method public static bad()V
    .locals 04

    const-string v0, "xydux ---"
    filled-new-array {v0}, [Ljava/lang/Cloneable;
    move-result-object v1
    invoke-static {v1}, LL/util;->print(Ljava/lang/Object;)V

    return-void
.end method


.method public static testInstanceOf(Ljava/util/AbstractMap;)Z
    .locals 01

    move-object v0, p0
    instance-of p0, v0, Ljava/util/TreeMap;
    if-eqz p0, :end2
        #return p0
        const p0, 1
:end2
    return p0
.end method


