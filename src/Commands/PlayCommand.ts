import { Commands } from './Commands';
import { UnicastMpv } from '../UnicastMpv';
import { string, tuple, optional } from '../Schema';
import { Server } from 'http';

export class PlayCommand extends Commands {
    constructor ( server : UnicastMpv ) {
        super( server );

        this.register( 'play', tuple( [ string(), optional( string() ) ] ), this.play.bind( this ) );
    }

    public async play ( file : string, subtitles : string ) {
        if ( this.server.player.config.get( 'restartOnPlay' ) == true ) {
            this.server.player.mpv.quit();
        }

        if ( !this.server.player.mpv.isRunning() ) {
            await this.server.player.start();
        }
        
        await this.server.player.mpv.load( file );

        if ( subtitles ) {
            await this.server.player.mpv.addSubtitles( subtitles );
        }
    }
}