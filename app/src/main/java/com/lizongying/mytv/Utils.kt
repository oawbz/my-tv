package com.lizongying.mytv

import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

object Utils {
    fun getDateFormat(format: String): String = SimpleDateFormat(format, Locale.CHINA).format(Date())

}
