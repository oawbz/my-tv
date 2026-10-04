package com.lizongying.mytv

import android.graphics.Color
import android.graphics.drawable.GradientDrawable
import android.util.TypedValue
import android.view.GestureDetector
import android.view.MotionEvent
import android.view.View
import android.view.ViewGroup
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import com.lizongying.mytv.models.TVViewModel
import kotlin.math.abs
import kotlin.math.min

/** Touch controls used only while mobile mode is enabled. Playback stays in PlayerFragment. */
class MobileControls(
    private val activity: MainActivity,
    private val root: View,
) {
    private val drawer: View = root.findViewById(R.id.mobile_drawer)
    private val scrim: View = root.findViewById(R.id.mobile_scrim)
    private val groupsView: LinearLayout = root.findViewById(R.id.mobile_groups)
    private val channelsView: LinearLayout = root.findViewById(R.id.mobile_channels)
    private val channelScroll: ScrollView = root.findViewById(R.id.mobile_channel_scroll)
    private var groups: List<Pair<String, List<TVViewModel>>> = emptyList()
    private var selectedGroup: String? = null
    private var currentChannel: TVViewModel? = null
    init {
        val maxWidth = dp(420)
        val availableWidth = activity.resources.displayMetrics.widthPixels
        drawer.layoutParams = drawer.layoutParams.apply {
            width = min(maxWidth, (availableWidth * 0.7f).toInt())
        }
        installSurface(root.findViewById(R.id.mobile_video_tap)) { openDrawer() }
        installSurface(scrim) { closeDrawer() }
        root.findViewById<View>(R.id.mobile_close_drawer).setOnClickListener { closeDrawer() }
    }

    fun setEnabled(enabled: Boolean) {
        root.visibility = if (enabled) View.VISIBLE else View.GONE
        closeAll()
    }

    fun setGroups(value: List<Pair<String, List<TVViewModel>>>) {
        groups = value
        if (selectedGroup !in groups.map { it.first }) selectedGroup = groups.firstOrNull()?.first
        if (drawer.visibility == View.VISIBLE) renderGroups()
    }

    fun setChannel(channel: TVViewModel) {
        currentChannel = channel
        if (drawer.visibility == View.VISIBLE) {
            val playingGroup = channel.getTV().channel
            if (groups.any { it.first == playingGroup }) selectedGroup = playingGroup
            renderGroups()
            scrollCurrentIntoView()
        }
    }

    fun isDrawerOpen(): Boolean = drawer.visibility == View.VISIBLE

    fun closeOverlay(): Boolean {
        if (isDrawerOpen()) {
            closeDrawer()
            return true
        }
        return false
    }

    fun closeAll() {
        drawer.visibility = View.GONE
        scrim.visibility = View.GONE
    }

    private fun openDrawer() {
        selectedGroup = currentChannel?.getTV()?.channel
            ?.takeIf { group -> groups.any { it.first == group } }
            ?: selectedGroup ?: groups.firstOrNull()?.first
        renderGroups()
        scrim.visibility = View.VISIBLE
        drawer.visibility = View.VISIBLE
        scrollCurrentIntoView()
    }

    private fun closeDrawer() {
        drawer.visibility = View.GONE
        scrim.visibility = View.GONE
    }

    private fun renderGroups() {
        groupsView.removeAllViews()
        for ((group, _) in groups) {
            val row = row(group, group == selectedGroup)
            row.setOnClickListener {
                selectedGroup = group
                renderGroups()
                channelScroll.scrollTo(0, 0)
            }
            groupsView.addView(row)
        }
        renderChannels()
    }

    private fun renderChannels() {
        channelsView.removeAllViews()
        val channels = groups.firstOrNull { it.first == selectedGroup }?.second.orEmpty()
        for (channel in channels) {
            val playing = channel === currentChannel
            val label = if (playing) "▶ ${channel.getTV().title}" else channel.getTV().title
            val row = row(label, playing)
            row.setOnClickListener {
                activity.playMobileChannel(channel)
                closeDrawer()
            }
            channelsView.addView(row)
        }
    }

    private fun scrollCurrentIntoView() {
        val channels = groups.firstOrNull { it.first == selectedGroup }?.second.orEmpty()
        val index = channels.indexOf(currentChannel)
        if (index < 0) return
        channelScroll.post {
            val row = channelsView.getChildAt(index) ?: return@post
            val visibleTop = channelScroll.scrollY
            val visibleBottom = visibleTop + channelScroll.height
            if (row.top < visibleTop || row.bottom > visibleBottom) {
                channelScroll.smoothScrollTo(0, (row.top - channelScroll.height / 2).coerceAtLeast(0))
            }
        }
    }

    private fun installSurface(surface: View, onTap: () -> Unit) {
        var startX = 0f
        var startY = 0f
        var longPressed = false
        surface.setOnClickListener { onTap() }
        surface.setOnLongClickListener {
            closeDrawer()
            activity.openMobileSettings()
            true
        }
        val detector = GestureDetector(activity, object : GestureDetector.SimpleOnGestureListener() {
            override fun onDown(e: MotionEvent): Boolean = true

            override fun onLongPress(e: MotionEvent) {
                longPressed = true
                surface.performLongClick()
            }
        })
        surface.setOnTouchListener { view, event ->
            if (event.actionMasked == MotionEvent.ACTION_DOWN) {
                startX = event.x
                startY = event.y
                longPressed = false
            }
            detector.onTouchEvent(event)
            if (event.actionMasked == MotionEvent.ACTION_UP && !longPressed) {
                val dx = event.x - startX
                val dy = event.y - startY
                if (SP.mobileSwipeChannel && abs(dy) >= dp(48) && abs(dy) > abs(dx) * 1.3f) {
                    if (dy < 0) activity.next() else activity.prev()
                } else if (abs(dx) < dp(12) && abs(dy) < dp(12)) {
                    view.performClick()
                }
            }
            true
        }
    }

    private fun row(label: String, selected: Boolean): TextView = TextView(activity).apply {
        text = label
        setTextColor(Color.WHITE)
        setTextSize(TypedValue.COMPLEX_UNIT_SP, 16f)
        gravity = android.view.Gravity.CENTER_VERTICAL
        minHeight = dp(52)
        setPadding(dp(12), dp(8), dp(8), dp(8))
        isSingleLine = true
        ellipsize = android.text.TextUtils.TruncateAt.END
        background = GradientDrawable().apply {
            setColor(if (selected) Color.rgb(59, 77, 99) else Color.TRANSPARENT)
        }
        layoutParams = LinearLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT,
            ViewGroup.LayoutParams.WRAP_CONTENT,
        )
    }

    private fun dp(value: Int): Int =
        (value * activity.resources.displayMetrics.density + 0.5f).toInt()
}
