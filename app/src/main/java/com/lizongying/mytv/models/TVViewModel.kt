package com.lizongying.mytv.models

import com.lizongying.mytv.TV

class TVViewModel(private val tv: TV) {
    private var sourceIndex = 0

    fun resetSource() { sourceIndex = 0 }
    fun nextSource(): Boolean {
        if (sourceIndex + 1 >= tv.videoUrl.size) return false
        sourceIndex++
        return true
    }
    fun getVideoUrlCurrent(): String = tv.videoUrl[sourceIndex]

    fun getTV(): TV = tv
}
