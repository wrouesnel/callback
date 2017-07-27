import React from 'react';
import {connect} from 'cerebral/react';
import {state} from 'cerebral/tags';

import moment from 'moment';
moment.locale('en-AU'); //FIXME: get the window locale


export default connect({
    },
    class CallbackSessions extends React.Component {
        constructor() {
            super();
        }
        render() {
            return (
                <div className="sessionlist">
                    <h1>Client Sessions</h1>
                    <table className="table">
                        <thead>
                            <tr>
                                <th>Client IP</th>
                                <th>Target ID</th>
                                <th>Establishment Time</th>
                                <th>Bytes Total</th>
                                <th>Bandwidth Usage</th>
                            </tr>
                        </thead>
                        <tbody>
                        </tbody>
                    </table>
                </div>
            )
        }
    }
);
