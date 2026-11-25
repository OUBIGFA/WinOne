#include "recording_controller.h"
#include <iostream>

int wmain(int argc,wchar_t** argv) {
    using namespace winone::capture;
    try {
        if(argc!=2)throw std::runtime_error("Pass the existing recording runtime directory");
        RecordingController engine{std::filesystem::path(argv[1])};
        if(engine.command("status").at("phase")!="idle" || engine.active())throw std::runtime_error("Native recording ABI did not report its initial state");
        for(const auto* method:{"pause_recording","resume_recording","stop_recording","cancel_recording"}){
            bool rejected=false;try{engine.command(method);}catch(const std::exception&){rejected=true;}
            if(!rejected)throw std::runtime_error("Recording action accepted without a session");
        }
        const json invalid{{"x",0},{"y",0},{"width",460},{"height",280},{"monitor","invalid"},{"output","invalid.gif"},{"format","gif"},{"fps",31}};
        bool rejected=false;try{engine.command("start_recording",invalid);}catch(const std::exception&){rejected=true;}
        if(!rejected || engine.command("status").at("phase")!="idle")throw std::runtime_error("Invalid native options started a recording");
        if(engine.communityState()!=tools3000::capture::RecordState::Idle || engine.communityStats().frameCount!=0)
            throw std::runtime_error("Community indicator state differs from Rust recording state");
        std::cout<<"PASS: actual Rust recording ABI, initial state, all lifecycle guards, invalid GIF options and HUD status mapping\n";
        return 0;
    }catch(const std::exception& error){std::cerr<<error.what()<<'\n';return 1;}
}
