#!/usr/bin/env node

import { UnicastMpv, Config } from './index';

const configs = [
    UnicastMpv.baseConfig()
];

if ( process.argv.length > 2 ) {
    configs.push( Config.load( process.argv[ 2 ] ) );
}

const server = new UnicastMpv( Config.merge( configs ) );

server.listen()
    .catch( error => server.logger.fatal( error.message + '\n' + error.stack, error ) );