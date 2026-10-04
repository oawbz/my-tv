package com.lizongying.mytv

import android.os.Bundle
import android.os.Handler
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import androidx.fragment.app.Fragment
import com.lizongying.mytv.databinding.InfoBinding
import com.lizongying.mytv.models.TVViewModel

class InfoFragment : Fragment() {
    private var _binding: InfoBinding? = null
    private val binding get() = _binding!!

    private val handler = Handler()
    private val delay: Long = 3000
    private var playbackActive = false

    override fun onCreateView(
        inflater: LayoutInflater, container: ViewGroup?,
        savedInstanceState: Bundle?
    ): View {
        _binding = InfoBinding.inflate(inflater, container, false)
        _binding!!.root.visibility = View.GONE
        (activity as MainActivity).fragmentReady("InfoFragment")
        return binding.root
    }

    fun show(tvViewModel: TVViewModel) {
        val binding = _binding ?: return
        binding.textView.text = tvViewModel.getTV().title

        val logo = tvViewModel.getTV().logo
        LogoLoader.load(binding.infoLogo, logo)

        handler.removeCallbacks(removeRunnable)
        binding.root.visibility = View.VISIBLE
        if (playbackActive) handler.postDelayed(removeRunnable, delay)
    }

    fun setPlaybackActive(active: Boolean) {
        playbackActive = active
        handler.removeCallbacks(removeRunnable)
        if (active) {
            if (isResumed) handler.postDelayed(removeRunnable, delay)
        } else {
            _binding?.root?.visibility = View.VISIBLE
        }
    }

    override fun onResume() {
        super.onResume()
        if (playbackActive) handler.postDelayed(removeRunnable, delay)
    }

    override fun onPause() {
        super.onPause()
        handler.removeCallbacks(removeRunnable)
    }

    private val removeRunnable = Runnable {
        if (playbackActive) _binding?.root?.visibility = View.GONE
    }

    override fun onDestroyView() {
        handler.removeCallbacks(removeRunnable)
        (activity as? MainActivity)?.fragmentUnavailable("InfoFragment")
        _binding?.infoLogo?.let(LogoLoader::clear)
        super.onDestroyView()
        _binding = null
    }
}
