import { TupleTypeSchema, SchemaValidationError } from '../Schema';
import { UnicastMpv } from '../UnicastMpv';

export class Commands {
    server : UnicastMpv;

    constructor ( server : UnicastMpv ) {
        this.server = server;
    }

    registerNative ( name : string, rawSchema : any[] | TupleTypeSchema = new TupleTypeSchema( [] ) ) {
        const method = name in this.server.player 
            ? this.server.player[ name ].bind( this.server.player )
            : this.server.player.mpv[ name ].bind( this.server.player.mpv );

        return this.register( name, rawSchema, ( ...args ) => method( ...args ) );
    }

    register ( name : string, rawSchema : any[] | TupleTypeSchema = new TupleTypeSchema( [] ), fn : ( ...args : any[] ) => any ) {
        let schema = rawSchema instanceof Array
            ? new TupleTypeSchema( rawSchema )
            : rawSchema;

        this.server.register( name, async ( args : any[] ) => {
            const errors = schema.validate( args );

            if ( errors != null ) {
                this.server.logger.service( name ).error( SchemaValidationError.toString( errors ) );

                return Promise.reject( errors );
            }

            return fn( ...args );
        } );
    }
}