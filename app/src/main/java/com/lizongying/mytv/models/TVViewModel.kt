package com.lizongying.mytv.models

import com.lizongying.mytv.TV

class TVViewModel(private val tv: TV) {
    private var rowPosition = 0
    private var itemPosition = 0
    private var sourceIndex = 0

    fun resetSource() { sourceIndex = 0 }
    fun nextSource(): Boolean {
        if (sourceIndex + 1 >= tv.videoUrl.size) return false
        sourceIndex++
        return true
    }
    fun getVideoUrlCurrent(): String = tv.videoUrl[sourceIndex]

    fun getRowPosition(): Int = rowPosition
    fun getItemPosition(): Int = itemPosition
    fun setRowPosition(position: Int) { rowPosition = position }
    fun setItemPosition(position: Int) { itemPosition = position }
    fun getTV(): TV = tv
}
